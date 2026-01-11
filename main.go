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
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
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

// PatternEntry represents a pattern in the TOML config file
type PatternEntry struct {
	Pattern string `toml:"pattern"`
	Name    string `toml:"name"`
}

// WrapperConfig represents the wrappers.toml file structure
type WrapperConfig struct {
	Wrapper []PatternEntry `toml:"wrapper"`
}

// CommandConfig represents the commands.toml file structure
type CommandConfig struct {
	Command []PatternEntry `toml:"command"`
}

//go:embed config/wrappers.toml
var defaultWrappers []byte

//go:embed config/commands.toml
var defaultCommands []byte

// Safe wrappers that can prefix any safe command (initialized by initConfig)
var wrapperPatterns []Pattern

// Safe core command patterns (initialized by initConfig)
var safeCommands []Pattern

// configInitialized tracks whether config has been loaded
var configInitialized bool

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

// getConfigDir returns the config directory path.
// Uses MMI_CONFIG env var if set, otherwise ~/.config/mmi
func getConfigDir() (string, error) {
	if dir := os.Getenv("MMI_CONFIG"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "mmi"), nil
}

// ensureConfigFiles creates the config directory and writes default config files if they don't exist.
func ensureConfigFiles(configDir string) error {
	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write default wrappers.toml if it doesn't exist
	wrappersPath := filepath.Join(configDir, "wrappers.toml")
	if _, err := os.Stat(wrappersPath); os.IsNotExist(err) {
		if err := os.WriteFile(wrappersPath, defaultWrappers, 0644); err != nil {
			return fmt.Errorf("failed to write wrappers.toml: %w", err)
		}
	}

	// Write default commands.toml if it doesn't exist
	commandsPath := filepath.Join(configDir, "commands.toml")
	if _, err := os.Stat(commandsPath); os.IsNotExist(err) {
		if err := os.WriteFile(commandsPath, defaultCommands, 0644); err != nil {
			return fmt.Errorf("failed to write commands.toml: %w", err)
		}
	}

	return nil
}

// loadPatterns loads patterns from a TOML file and compiles them to regex.
func loadPatterns(data []byte, key string) ([]Pattern, error) {
	var config map[string][]PatternEntry
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	entries := config[key]
	patterns := make([]Pattern, 0, len(entries))
	for _, entry := range entries {
		re, err := regexp.Compile(entry.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", entry.Pattern, err)
		}
		patterns = append(patterns, Pattern{Regex: re, Name: entry.Name})
	}
	return patterns, nil
}

// loadEmbeddedDefaults loads patterns from the embedded default config files.
func loadEmbeddedDefaults() {
	wrapperPatterns, _ = loadPatterns(defaultWrappers, "wrapper")
	safeCommands, _ = loadPatterns(defaultCommands, "command")
}

// initConfig loads configuration from files, creating defaults if necessary.
// It sets wrapperPatterns and safeCommands globals.
// If loading fails, it falls back to embedded defaults.
func initConfig() error {
	if configInitialized {
		return nil
	}

	configDir, err := getConfigDir()
	if err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return err
	}

	if err := ensureConfigFiles(configDir); err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return err
	}

	// Load wrappers
	wrappersPath := filepath.Join(configDir, "wrappers.toml")
	wrappersData, err := os.ReadFile(wrappersPath)
	if err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to read wrappers.toml: %w", err)
	}

	wrapperPatterns, err = loadPatterns(wrappersData, "wrapper")
	if err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to load wrapper patterns: %w", err)
	}

	// Load commands
	commandsPath := filepath.Join(configDir, "commands.toml")
	commandsData, err := os.ReadFile(commandsPath)
	if err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to read commands.toml: %w", err)
	}

	safeCommands, err = loadPatterns(commandsData, "command")
	if err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to load command patterns: %w", err)
	}

	configInitialized = true
	return nil
}

func main() {
	initConfig() // Errors are ignored; fallbacks are used if config fails
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
