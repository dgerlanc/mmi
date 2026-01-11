// mmi (mother may I?) - Claude Code PreToolUse Hook for Bash Command Approval
//
// This hook auto-approves Bash commands that are safe combinations of:
//   WRAPPERS (timeout, env vars, .venv/bin/) + CORE COMMANDS (git, pytest, etc.)
//
// Usage in ~/.claude/settings.json:
//
//	"hooks": {
//	  "PreToolUse": [{
//	    "matcher": "Bash",
//	    "hooks": [{"type": "command", "command": "mmi"}]
//	  }]
//	}
//
// Test:
//
//	echo '{"tool_name": "Bash", "tool_input": {"command": "timeout 30 pytest"}}' | mmi
package main

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
)

// HookInput represents the JSON input from Claude Code
type HookInput struct {
	ToolName  string            `json:"tool_name"`
	ToolInput map[string]string `json:"tool_input"`
}

// HookOutput represents the approval JSON output
type HookOutput struct {
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput"`
}

// HookSpecificOutput contains the permission decision
type HookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// Pattern holds a compiled regex and its description
type Pattern struct {
	Regex *regexp.Regexp
	Name  string
}

// Safe wrappers that can prefix any safe command
var wrapperPatterns = []Pattern{
	{regexp.MustCompile(`^timeout\s+\d+\s+`), "timeout"},
	{regexp.MustCompile(`^nice\s+(-n\s*\d+\s+)?`), "nice"},
	{regexp.MustCompile(`^env\s+`), "env"},
	{regexp.MustCompile(`^([A-Z_][A-Z0-9_]*=[^\s]*\s+)+`), "env vars"},
	// Virtual env paths: .venv/bin/, ../.venv/bin/, /abs/path/.venv/bin/, venv/bin/
	{regexp.MustCompile(`^(\.\./)*\.?venv/bin/`), ".venv"},
	{regexp.MustCompile(`^/[^\s]+/\.?venv/bin/`), ".venv"},
	// do (loop body prefix)
	{regexp.MustCompile(`^do\s+`), "do"},
}

// Safe core command patterns
var safeCommands = []Pattern{
	// git read operations (with optional -C flag)
	{regexp.MustCompile(`^git\s+(-C\s+\S+\s+)?(diff|log|status|show|branch|stash\s+list|bisect|worktree\s+list|fetch)\b`), "git read op"},
	// git write operations
	{regexp.MustCompile(`^git\s+(-C\s+\S+\s+)?(add|checkout|merge|rebase|stash)\b`), "git write op"},
	// pytest
	{regexp.MustCompile(`^pytest\b`), "pytest"},
	// python
	{regexp.MustCompile(`^python\b`), "python"},
	// ruff (python linter/formatter)
	{regexp.MustCompile(`^ruff\b`), "ruff"},
	// uv / uvx
	{regexp.MustCompile(`^uv\s+(pip|run|sync|venv|add|remove|lock)\b`), "uv"},
	{regexp.MustCompile(`^uvx\b`), "uvx"},
	// npm / npx
	{regexp.MustCompile(`^npm\s+(install|run|test|build|ci)\b`), "npm"},
	{regexp.MustCompile(`^npx\b`), "npx"},
	// cargo
	{regexp.MustCompile(`^cargo\s+(build|test|run|check|clippy|fmt|clean)\b`), "cargo"},
	// maturin (rust python bindings)
	{regexp.MustCompile(`^maturin\s+(develop|build)\b`), "maturin"},
	// make
	{regexp.MustCompile(`^make\b`), "make"},
	// common read-only commands
	{regexp.MustCompile(`^(ls|cat|head|tail|wc|find|grep|rg|file|which|pwd|du|df|curl|sort|uniq|cut|tr|awk|sed|xargs)\b`), "read-only"},
	// touch (update timestamps, create empty files)
	{regexp.MustCompile(`^touch\b`), "touch"},
	// shell builtins for control flow
	{regexp.MustCompile(`^(true|false|exit(\s+\d+)?)$`), "shell builtin"},
	// pkill/kill (process management)
	{regexp.MustCompile(`^(pkill|kill)\b`), "process mgmt"},
	// echo (often used for logging/separators in chained commands)
	{regexp.MustCompile(`^echo\b`), "echo"},
	// cd (change directory, often first in a chain)
	{regexp.MustCompile(`^cd\s`), "cd"},
	// source/. (activate scripts, set env)
	{regexp.MustCompile(`^(source|\.) [^\s]*venv/bin/activate`), "venv activate"},
	// sleep (delays, often used in scripts)
	{regexp.MustCompile(`^sleep\s`), "sleep"},
	// variable assignment (VAR=value, VAR=$!, etc.)
	{regexp.MustCompile(`^[A-Z_][A-Z0-9_]*=\S*$`), "var assignment"},
	// for/while loops and loop constructs
	{regexp.MustCompile(`^for\s+\w+\s+in\s`), "for loop"},
	{regexp.MustCompile(`^while\s`), "while loop"},
	{regexp.MustCompile(`^done$`), "done"},
}

// Regex for dangerous constructs
var dangerousPattern = regexp.MustCompile(`\$\(|` + "`")

// Regex for splitting command chains
var separatorPattern = regexp.MustCompile(`\s*(?:&&|\|\||;|\||&)\s*`)
var separatorOrNewlinePattern = regexp.MustCompile(`\s*(?:&&|\|\||;|\||&)\s*|\n`)

// Regex for backslash-newline continuations
var backslashNewlinePattern = regexp.MustCompile(`\\\n\s*`)

// Regex for quoted strings
var doubleQuotePattern = regexp.MustCompile(`"[^"]*"`)
var singleQuotePattern = regexp.MustCompile(`'[^']*'`)

// Regex for redirections
var redirPattern = regexp.MustCompile(`(\d*)>&(\d*)`)

// osExit is a variable so it can be mocked in tests
var osExit = os.Exit

func main() {
	osExit(run(os.Stdin, os.Stdout))
}

// run executes the hook logic and returns the exit code.
// It returns 0 in all cases (rejection is silent, approval outputs JSON).
func run(stdin io.Reader, stdout io.Writer) int {
	approved, reason := process(stdin)
	if approved {
		stdout.Write([]byte(formatApproval(reason)))
	}
	return 0
}

// process reads from r and returns whether the command should be approved and the reason.
// Returns false for parse errors, non-Bash tools, dangerous patterns, or unsafe commands.
func process(r io.Reader) (approved bool, reason string) {
	var input HookInput
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		return false, ""
	}

	if input.ToolName != "Bash" {
		return false, ""
	}

	cmd := input.ToolInput["command"]

	// Reject dangerous constructs
	if dangerousPattern.MatchString(cmd) {
		return false, ""
	}

	segments := splitCommandChain(cmd)
	var reasons []string

	for _, segment := range segments {
		coreCmd, wrappers := stripWrappers(segment)
		r := checkSafe(coreCmd)
		if r == "" {
			return false, "" // One unsafe segment = reject entire command
		}
		if len(wrappers) > 0 {
			reasons = append(reasons, strings.Join(wrappers, "+")+" + "+r)
		} else {
			reasons = append(reasons, r)
		}
	}

	return true, strings.Join(reasons, " | ")
}

// formatApproval returns the JSON approval output with a trailing newline
func formatApproval(reason string) string {
	output := HookOutput{
		HookSpecificOutput: HookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "allow",
			PermissionDecisionReason: reason,
		},
	}
	data, _ := json.Marshal(output)
	return string(data) + "\n"
}

// splitCommandChain splits command into segments on &&, ||, ;, |
func splitCommandChain(cmd string) []string {
	// Collapse backslash-newline continuations
	cmd = backslashNewlinePattern.ReplaceAllString(cmd, " ")

	// Protect quoted strings from splitting (replace with placeholders)
	var quotedStrings []string
	saveQuoted := func(s string) string {
		quotedStrings = append(quotedStrings, s)
		return "__QUOTED_" + string(rune('0'+len(quotedStrings)-1)) + "__"
	}

	cmd = doubleQuotePattern.ReplaceAllStringFunc(cmd, saveQuoted)
	cmd = singleQuotePattern.ReplaceAllStringFunc(cmd, saveQuoted)

	// Normalize redirections to prevent splitting on & in 2>&1
	cmd = redirPattern.ReplaceAllString(cmd, "__REDIR_${1}_${2}__")
	cmd = strings.ReplaceAll(cmd, "&>", "__REDIR_AMPGT__")

	// Split on command separators
	var segments []string
	if len(quotedStrings) > 0 {
		segments = separatorPattern.Split(cmd, -1)
	} else {
		segments = separatorOrNewlinePattern.Split(cmd, -1)
	}

	// Restore quoted strings and redirections
	restore := func(s string) string {
		// Restore redirections
		s = regexp.MustCompile(`__REDIR_(\d*)_(\d*)__`).ReplaceAllString(s, "${1}>&${2}")
		s = strings.ReplaceAll(s, "__REDIR_AMPGT__", "&>")
		// Restore quoted strings
		for i, qs := range quotedStrings {
			placeholder := "__QUOTED_" + string(rune('0'+i)) + "__"
			s = strings.ReplaceAll(s, placeholder, qs)
		}
		return s
	}

	var result []string
	for _, seg := range segments {
		seg = strings.TrimSpace(restore(seg))
		if seg != "" {
			result = append(result, seg)
		}
	}
	return result
}

// stripWrappers strips safe wrapper prefixes, returns (core_cmd, list_of_wrappers)
func stripWrappers(cmd string) (string, []string) {
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

// checkSafe checks if command matches a safe pattern, returns reason or empty string
func checkSafe(cmd string) string {
	for _, p := range safeCommands {
		if p.Regex.MatchString(cmd) {
			return p.Name
		}
	}
	return ""
}
