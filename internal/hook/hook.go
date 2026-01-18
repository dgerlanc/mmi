// Package hook implements the core command approval logic for mmi.
package hook

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

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
	Reason   string // The reason for approval/denial
}

// ToolInputData represents the tool_input field in the Claude Code hook input
type ToolInputData struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"` // optional
	Timeout     int    `json:"timeout,omitempty"`     // optional
}

// Input represents the JSON input from Claude Code
type Input struct {
	SessionID      string        `json:"session_id"`
	TranscriptPath string        `json:"transcript_path"`
	Cwd            string        `json:"cwd"`
	PermissionMode string        `json:"permission_mode"`
	HookEventName  string        `json:"hook_event_name"`
	ToolName       string        `json:"tool_name"`
	ToolInput      ToolInputData `json:"tool_input"`
	ToolUseID      string        `json:"tool_use_id"`
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

// byteRange represents a range of bytes in a string
type byteRange struct {
	start, end int
}

// findQuotedHeredocRanges parses a command and returns byte ranges of heredoc content
// where the delimiter is quoted (single or double quotes). Quoted heredocs don't perform
// shell expansion, so backticks and $() inside them are literal text, not command substitution.
func findQuotedHeredocRanges(cmd string) []byteRange {
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil
	}

	var ranges []byteRange
	syntax.Walk(prog, func(node syntax.Node) bool {
		redir, ok := node.(*syntax.Redirect)
		if !ok {
			return true
		}

		// Check if this is a heredoc operator (<< or <<-)
		if redir.Op != syntax.Hdoc && redir.Op != syntax.DashHdoc {
			return true
		}

		// Check if the delimiter is quoted
		if redir.Word == nil || len(redir.Word.Parts) == 0 {
			return true
		}

		isQuoted := false
		for _, part := range redir.Word.Parts {
			switch part.(type) {
			case *syntax.SglQuoted, *syntax.DblQuoted:
				isQuoted = true
			}
		}

		// If quoted and has heredoc content, record the range
		if isQuoted && redir.Hdoc != nil {
			start := int(redir.Hdoc.Pos().Offset())
			end := int(redir.Hdoc.End().Offset())
			if start < end && start >= 0 && end <= len(cmd) {
				ranges = append(ranges, byteRange{start: start, end: end})
			}
		}

		return true
	})

	return ranges
}

// containsDangerousPattern checks if the command contains dangerous patterns ($( or backticks)
// while excluding content inside quoted heredocs where these characters are literal.
func containsDangerousPattern(cmd string) bool {
	excludeRanges := findQuotedHeredocRanges(cmd)

	// If no heredocs, do the simple check
	if len(excludeRanges) == 0 {
		return dangerousPattern.MatchString(cmd)
	}

	// Find all matches of the dangerous pattern
	matches := dangerousPattern.FindAllStringIndex(cmd, -1)
	if matches == nil {
		return false
	}

	// Check if any match is outside the excluded ranges
	for _, match := range matches {
		matchStart := match[0]
		inExcludedRange := false
		for _, r := range excludeRanges {
			if matchStart >= r.start && matchStart < r.end {
				inExcludedRange = true
				break
			}
		}
		if !inExcludedRange {
			return true
		}
	}

	return false
}

// Read a command and return whether it should be approved and the reason.
// Returns false for parse errors, non-Bash tools, dangerous patterns, or unsafe commands.
func Process(r io.Reader) (approved bool, reason string) {
	result := ProcessWithResult(r)
	return result.Approved, result.Reason
}

// ProcessWithResult reads from a stream and returns a Result with full details.
// This is useful when the caller needs the original command for logging.
func ProcessWithResult(r io.Reader) Result {
	startTime := time.Now()

	var input Input
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		logger.Debug("failed to decode input", "error", err)
		return Result{}
	}

	if input.ToolName != "Bash" {
		logger.Debug("not a Bash command", "tool", input.ToolName)
		return Result{}
	}

	cmd := input.ToolInput.Command
	logger.Debug("processing command", "command", cmd)

	// Reject dangerous constructs (command substitution outside quoted heredocs)
	if containsDangerousPattern(cmd) {
		logger.Debug("rejected dangerous pattern", "command", cmd)
		durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0
		segments := []audit.Segment{{
			Command:   cmd,
			Approved:  false,
			Rejection: &audit.Rejection{Code: audit.CodeCommandSubstitution, Pattern: "$(...)"},
		}}
		logAudit(cmd, false, segments, durationMs, input.SessionID, input.ToolUseID, input.Cwd)
		return Result{Command: cmd, Approved: false}
	}

	cfg := config.Get()

	cmdSegments, err := SplitCommandChain(cmd)
	if err != nil {
		logger.Debug("rejected unparseable command", "command", cmd)
		durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0
		segments := []audit.Segment{{
			Command:   cmd,
			Approved:  false,
			Rejection: &audit.Rejection{Code: audit.CodeUnparseable, Detail: "parse error"},
		}}
		logAudit(cmd, false, segments, durationMs, input.SessionID, input.ToolUseID, input.Cwd)
		return Result{Command: cmd, Approved: false, Reason: "unparseable command"}
	}
	logger.Debug("split command chain", "segments", len(cmdSegments))

	var reasons []string
	var auditSegments []audit.Segment

	for i, segment := range cmdSegments {
		coreCmd, wrappers := StripWrappers(segment, cfg.WrapperPatterns)
		logger.Debug("processing segment",
			"index", i,
			"segment", segment,
			"core", coreCmd,
			"wrappers", wrappers)

		// Check deny list on core command (after splitting chain and stripping wrappers)
		denyResult := CheckDenyWithResult(coreCmd, cfg.DenyPatterns)
		if denyResult.Denied {
			logger.Debug("rejected by deny list", "command", coreCmd, "reason", denyResult.Name)
			durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0
			// Add already processed segments plus the denied one
			auditSegments = append(auditSegments, audit.Segment{
				Command:  segment,
				Approved: false,
				Wrappers: wrappers,
				Rejection: &audit.Rejection{
					Code:    audit.CodeDenyMatch,
					Name:    denyResult.Name,
					Pattern: denyResult.Pattern,
				},
			})
			logAudit(cmd, false, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd)
			return Result{Command: cmd, Approved: false}
		}

		safeResult := CheckSafeWithResult(coreCmd, cfg.SafeCommands)
		if !safeResult.Matched {
			logger.Debug("rejected unsafe command", "command", coreCmd)
			durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0
			// Add already processed segments plus the rejected one
			auditSegments = append(auditSegments, audit.Segment{
				Command:   segment,
				Approved:  false,
				Wrappers:  wrappers,
				Rejection: &audit.Rejection{Code: audit.CodeNoMatch},
			})
			logAudit(cmd, false, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd)
			return Result{Command: cmd, Approved: false}
		}
		logger.Debug("matched pattern", "command", coreCmd, "pattern", safeResult.Name)

		// Add approved segment
		auditSegments = append(auditSegments, audit.Segment{
			Command:  segment,
			Approved: true,
			Wrappers: wrappers,
			Match: &audit.Match{
				Type:    safeResult.Type,
				Name:    safeResult.Name,
				Pattern: safeResult.Pattern,
			},
		})

		if len(wrappers) > 0 {
			reasons = append(reasons, strings.Join(wrappers, "+")+" + "+safeResult.Name)
		} else {
			reasons = append(reasons, safeResult.Name)
		}
	}

	reason := strings.Join(reasons, " | ")
	logger.Debug("approved", "reason", reason)
	durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0
	logAudit(cmd, true, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd)
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

// SafeResult contains detailed information about a safe pattern match.
type SafeResult struct {
	Matched bool
	Name    string
	Type    string // simple, subcommand, regex, command
	Pattern string
}

// CheckSafeWithResult checks if a command matches a safe pattern and returns details.
func CheckSafeWithResult(cmd string, safeCommands []patterns.Pattern) SafeResult {
	for _, p := range safeCommands {
		if p.Regex.MatchString(cmd) {
			return SafeResult{
				Matched: true,
				Name:    p.Name,
				Type:    p.Type,
				Pattern: p.Pattern,
			}
		}
	}
	return SafeResult{Matched: false}
}

// DenyResult contains detailed information about a deny pattern match.
type DenyResult struct {
	Denied  bool
	Name    string
	Pattern string
}

// CheckDenyWithResult checks if a command matches a deny pattern and returns details.
func CheckDenyWithResult(cmd string, denyPatterns []patterns.Pattern) DenyResult {
	for _, p := range denyPatterns {
		if p.Regex.MatchString(cmd) {
			return DenyResult{
				Denied:  true,
				Name:    p.Name,
				Pattern: p.Pattern,
			}
		}
	}
	return DenyResult{Denied: false}
}

// logAudit logs a command decision to the audit log.
func logAudit(command string, approved bool, segments []audit.Segment, durationMs float64, sessionID, toolUseID, cwd string) {
	audit.Log(audit.Entry{
		Version:    1,
		SessionID:  sessionID,
		ToolUseID:  toolUseID,
		Command:    command,
		Approved:   approved,
		Segments:   segments,
		DurationMs: durationMs,
		Cwd:        cwd,
	})
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

// FormatAsk returns the JSON ask output with a trailing newline
func FormatAsk(reason string) string {
	output := Output{
		HookSpecificOutput: SpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "ask",
			PermissionDecisionReason: reason,
		},
	}
	data, _ := json.Marshal(output)
	return string(data) + "\n"
}

// ErrUnparseable is returned when a command cannot be parsed.
var ErrUnparseable = errors.New("unparseable command")

// SplitCommandChain splits command into segments on &&, ||, ;, |, & using a proper shell parser.
// This handles quoted strings, redirections, and other shell syntax correctly.
// Returns ErrUnparseable if the command cannot be parsed.
func SplitCommandChain(cmd string) ([]string, error) {
	if strings.TrimSpace(cmd) == "" {
		return nil, nil
	}

	// Parse the command using the shell parser
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil, ErrUnparseable
	}

	var segments []string
	printer := syntax.NewPrinter()

	// Walk the AST to extract individual commands
	for _, stmt := range prog.Stmts {
		extractCommands(stmt.Cmd, printer, &segments)
	}

	return segments, nil
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
