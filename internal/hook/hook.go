// Package hook implements the core command approval logic for mmi.
package hook

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/dgerlanc/mmi/internal/audit"
	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/logger"
	"github.com/dgerlanc/mmi/internal/patterns"
	"mvdan.cc/sh/v3/syntax"
)

// Result contains the outcome of processing a command.
type Result struct {
	Command  string // The command that was processed
	Approved bool   // Whether the command was approved
	Reason   string // The reason for approval (empty if rejected)
}

// Input represents the JSON input from Claude Code
type Input struct {
	ToolName  string            `json:"tool_name"`
	ToolInput map[string]string `json:"tool_input"`
}

// Output represents the approval JSON output
type Output struct {
	HookSpecificOutput SpecificOutput `json:"hookSpecificOutput"`
}

// SpecificOutput contains the permission decision
type SpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// subshellPattern matches $(...) command substitution syntax
var subshellPattern = regexp.MustCompile(`\$\(`)

// subshellExtractPattern extracts the command inside $(...) - captures the first word
var subshellExtractPattern = regexp.MustCompile(`\$\(([a-zA-Z0-9_-]+)`)

// backtickPattern matches `...` command substitution syntax
var backtickPattern = regexp.MustCompile("`")

// backtickExtractPattern extracts the command inside backticks - captures the first word
var backtickExtractPattern = regexp.MustCompile("`([a-zA-Z0-9_-]+)")

// currentProfile holds the current profile name for audit logging
var currentProfile string

// SetProfile sets the current profile name for audit logging.
func SetProfile(profile string) {
	currentProfile = profile
}

// Process reads from r and returns whether the command should be approved and the reason.
// Returns false for parse errors, non-Bash tools, dangerous patterns, or unsafe commands.
func Process(r io.Reader) (approved bool, reason string) {
	result := ProcessWithResult(r)
	return result.Approved, result.Reason
}

// ProcessWithResult reads from r and returns a Result with full details.
// This is useful when the caller needs the original command for logging.
func ProcessWithResult(r io.Reader) Result {
	var input Input
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		logger.Debug("failed to decode input", "error", err)
		return Result{}
	}

	if input.ToolName != "Bash" {
		logger.Debug("not a Bash command", "tool", input.ToolName)
		return Result{}
	}

	cmd := input.ToolInput["command"]
	logger.Debug("processing command", "command", cmd)

	cfg := config.Get()

	// Check for dangerous constructs (command substitution) based on config
	if subshellPattern.MatchString(cmd) {
		if !checkSubshellsAllowed(cmd, cfg.General.AllowSubshells, cfg.General.AllowedSubshellCommands, subshellExtractPattern) {
			logger.Debug("rejected subshell pattern", "command", cmd)
			logAudit(cmd, false, "")
			return Result{Command: cmd, Approved: false}
		}
	}
	if backtickPattern.MatchString(cmd) {
		if !checkSubshellsAllowed(cmd, cfg.General.AllowBackticks, cfg.General.AllowedSubshellCommands, backtickExtractPattern) {
			logger.Debug("rejected backtick pattern", "command", cmd)
			logAudit(cmd, false, "")
			return Result{Command: cmd, Approved: false}
		}
	}

	// Check deny list FIRST (before any approval checks)
	if denyReason := CheckDeny(cmd, cfg.DenyPatterns); denyReason != "" {
		logger.Debug("rejected by deny list", "command", cmd, "reason", denyReason)
		logAudit(cmd, false, "")
		return Result{Command: cmd, Approved: false}
	}

	segments := SplitCommandChain(cmd)
	logger.Debug("split command chain", "segments", len(segments))

	var reasons []string

	for i, segment := range segments {
		// Check deny list for each segment too
		if denyReason := CheckDeny(segment, cfg.DenyPatterns); denyReason != "" {
			logger.Debug("segment rejected by deny list", "segment", segment, "reason", denyReason)
			logAudit(cmd, false, "")
			return Result{Command: cmd, Approved: false}
		}

		coreCmd, wrappers := StripWrappers(segment, cfg.WrapperPatterns)
		logger.Debug("processing segment",
			"index", i,
			"segment", segment,
			"core", coreCmd,
			"wrappers", wrappers)

		// Check deny list for core command after stripping wrappers
		if denyReason := CheckDeny(coreCmd, cfg.DenyPatterns); denyReason != "" {
			logger.Debug("core command rejected by deny list", "command", coreCmd, "reason", denyReason)
			logAudit(cmd, false, "")
			return Result{Command: cmd, Approved: false}
		}

		r := CheckSafe(coreCmd, cfg.SafeCommands)
		if r == "" {
			logger.Debug("rejected unsafe command", "command", coreCmd)
			logAudit(cmd, false, "")
			return Result{Command: cmd, Approved: false}
		}
		logger.Debug("matched pattern", "command", coreCmd, "pattern", r)

		if len(wrappers) > 0 {
			reasons = append(reasons, strings.Join(wrappers, "+")+" + "+r)
		} else {
			reasons = append(reasons, r)
		}
	}

	reason := strings.Join(reasons, " | ")
	logger.Debug("approved", "reason", reason)
	logAudit(cmd, true, reason)
	return Result{Command: cmd, Approved: true, Reason: reason}
}

// CheckDeny checks if a command matches any deny pattern.
// Returns the pattern name if denied, or empty string if not.
func CheckDeny(cmd string, denyPatterns []patterns.Pattern) string {
	for _, p := range denyPatterns {
		if p.Regex.MatchString(cmd) {
			return p.Name
		}
	}
	return ""
}

// logAudit logs a command decision to the audit log.
func logAudit(command string, approved bool, reason string) {
	audit.Log(audit.Entry{
		Command:  command,
		Approved: approved,
		Reason:   reason,
		Profile:  currentProfile,
	})
}

// checkSubshellsAllowed checks if command substitution is allowed.
// Returns true if:
// - allowAll is true (all subshells allowed), or
// - allowedCommands is non-empty and ALL extracted commands are in the list
// The extractPattern should capture the command name in group 1.
func checkSubshellsAllowed(cmd string, allowAll bool, allowedCommands []string, extractPattern *regexp.Regexp) bool {
	if allowAll {
		return true
	}

	// If no specific commands are allowed, reject
	if len(allowedCommands) == 0 {
		return false
	}

	// Extract all subshell commands and check each one
	matches := extractPattern.FindAllStringSubmatch(cmd, -1)
	if len(matches) == 0 {
		// Pattern matched but couldn't extract command - reject for safety
		return false
	}

	// Build a set of allowed commands for fast lookup
	allowed := make(map[string]bool, len(allowedCommands))
	for _, c := range allowedCommands {
		allowed[c] = true
	}

	// All extracted commands must be in the allowed list
	for _, match := range matches {
		if len(match) < 2 {
			return false
		}
		extractedCmd := match[1]
		if !allowed[extractedCmd] {
			logger.Debug("subshell command not in allowed list", "command", extractedCmd, "allowed", allowedCommands)
			return false
		}
	}

	return true
}

// FormatApproval returns the JSON approval output with a trailing newline
func FormatApproval(reason string) string {
	output := Output{
		HookSpecificOutput: SpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "allow",
			PermissionDecisionReason: reason,
		},
	}
	data, _ := json.Marshal(output)
	return string(data) + "\n"
}

// SplitCommandChain splits command into segments on &&, ||, ;, |, & using a proper shell parser.
// This handles quoted strings, redirections, and other shell syntax correctly.
func SplitCommandChain(cmd string) []string {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}

	// Parse the command using the shell parser
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		// If parsing fails, fall back to treating it as a single command
		return []string{strings.TrimSpace(cmd)}
	}

	var segments []string
	printer := syntax.NewPrinter()

	// Walk the AST to extract individual commands
	for _, stmt := range prog.Stmts {
		extractCommands(stmt.Cmd, printer, &segments)
	}

	return segments
}

// extractCommands recursively extracts simple commands from a shell AST node.
func extractCommands(node syntax.Command, printer *syntax.Printer, segments *[]string) {
	if node == nil {
		return
	}

	switch cmd := node.(type) {
	case *syntax.CallExpr:
		var buf strings.Builder
		printer.Print(&buf, cmd)
		if s := strings.TrimSpace(buf.String()); s != "" {
			*segments = append(*segments, s)
		}

	case *syntax.BinaryCmd:
		extractCommands(cmd.X.Cmd, printer, segments)
		extractCommands(cmd.Y.Cmd, printer, segments)

	case *syntax.Subshell:
		for _, stmt := range cmd.Stmts {
			extractCommands(stmt.Cmd, printer, segments)
		}

	case *syntax.Block:
		for _, stmt := range cmd.Stmts {
			extractCommands(stmt.Cmd, printer, segments)
		}

	case *syntax.IfClause:
		for clause := cmd; clause != nil; clause = clause.Else {
			for _, stmt := range clause.Cond {
				extractCommands(stmt.Cmd, printer, segments)
			}
			for _, stmt := range clause.Then {
				extractCommands(stmt.Cmd, printer, segments)
			}
		}

	case *syntax.WhileClause:
		for _, stmt := range cmd.Cond {
			extractCommands(stmt.Cmd, printer, segments)
		}
		for _, stmt := range cmd.Do {
			extractCommands(stmt.Cmd, printer, segments)
		}

	case *syntax.ForClause:
		for _, stmt := range cmd.Do {
			extractCommands(stmt.Cmd, printer, segments)
		}

	case *syntax.CaseClause:
		for _, item := range cmd.Items {
			for _, stmt := range item.Stmts {
				extractCommands(stmt.Cmd, printer, segments)
			}
		}

	case *syntax.DeclClause:
		var buf strings.Builder
		printer.Print(&buf, cmd)
		if s := strings.TrimSpace(buf.String()); s != "" {
			*segments = append(*segments, s)
		}

	case *syntax.LetClause:
		var buf strings.Builder
		printer.Print(&buf, cmd)
		if s := strings.TrimSpace(buf.String()); s != "" {
			*segments = append(*segments, s)
		}

	case *syntax.TimeClause:
		if cmd.Stmt != nil {
			extractCommands(cmd.Stmt.Cmd, printer, segments)
		}

	case *syntax.CoprocClause:
		if cmd.Stmt != nil {
			extractCommands(cmd.Stmt.Cmd, printer, segments)
		}

	case *syntax.FuncDecl:
		if cmd.Body != nil {
			extractCommands(cmd.Body.Cmd, printer, segments)
		}

	case *syntax.ArithmCmd:
		var buf strings.Builder
		printer.Print(&buf, cmd)
		if s := strings.TrimSpace(buf.String()); s != "" {
			*segments = append(*segments, s)
		}

	case *syntax.TestClause:
		var buf strings.Builder
		printer.Print(&buf, cmd)
		if s := strings.TrimSpace(buf.String()); s != "" {
			*segments = append(*segments, s)
		}

	default:
		var buf strings.Builder
		printer.Print(&buf, cmd)
		if s := strings.TrimSpace(buf.String()); s != "" {
			*segments = append(*segments, s)
		}
	}
}

// StripWrappers strips safe wrapper prefixes from a command.
// Returns (core_cmd, list_of_wrapper_names)
func StripWrappers(cmd string, wrapperPatterns []patterns.Pattern) (string, []string) {
	var wrappers []string
	changed := true
	for changed {
		changed = false
		for _, p := range wrapperPatterns {
			loc := p.Regex.FindStringIndex(cmd)
			if loc != nil && loc[0] == 0 {
				wrappers = append(wrappers, p.Name)
				cmd = cmd[loc[1]:]
				changed = true
				break
			}
		}
	}
	return strings.TrimSpace(cmd), wrappers
}

// CheckSafe checks if a command matches a safe pattern.
// Returns the pattern name or empty string if not safe.
func CheckSafe(cmd string, safeCommands []patterns.Pattern) string {
	for _, p := range safeCommands {
		if p.Regex.MatchString(cmd) {
			return p.Name
		}
	}
	return ""
}
