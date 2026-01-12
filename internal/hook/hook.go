// Package hook implements the core command approval logic for mmi.
package hook

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/logger"
	"github.com/dgerlanc/mmi/patterns"
	"mvdan.cc/sh/v3/syntax"
)

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

// dangerousPattern matches command substitution syntax
var dangerousPattern = regexp.MustCompile(`\$\(|` + "`")

// Process reads from r and returns whether the command should be approved and the reason.
// Returns false for parse errors, non-Bash tools, dangerous patterns, or unsafe commands.
func Process(r io.Reader) (approved bool, reason string) {
	var input Input
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		logger.Debug("failed to decode input", "error", err)
		return false, ""
	}

	if input.ToolName != "Bash" {
		logger.Debug("not a Bash command", "tool", input.ToolName)
		return false, ""
	}

	cmd := input.ToolInput["command"]
	logger.Debug("processing command", "command", cmd)

	// Reject dangerous constructs
	if dangerousPattern.MatchString(cmd) {
		logger.Debug("rejected dangerous pattern", "command", cmd)
		return false, ""
	}

	segments := SplitCommandChain(cmd)
	logger.Debug("split command chain", "segments", len(segments))

	cfg := config.Get()
	var reasons []string

	for i, segment := range segments {
		coreCmd, wrappers := StripWrappers(segment, cfg.WrapperPatterns)
		logger.Debug("processing segment",
			"index", i,
			"segment", segment,
			"core", coreCmd,
			"wrappers", wrappers)

		r := CheckSafe(coreCmd, cfg.SafeCommands)
		if r == "" {
			logger.Debug("rejected unsafe command", "command", coreCmd)
			return false, "" // One unsafe segment = reject entire command
		}
		logger.Debug("matched pattern", "command", coreCmd, "pattern", r)

		if len(wrappers) > 0 {
			reasons = append(reasons, strings.Join(wrappers, "+")+" + "+r)
		} else {
			reasons = append(reasons, r)
		}
	}

	result := strings.Join(reasons, " | ")
	logger.Debug("approved", "reason", result)
	return true, result
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
