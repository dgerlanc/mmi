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

// RegexEntry represents a raw regex pattern in the config
type RegexEntry struct {
	Pattern string `toml:"pattern"`
	Name    string `toml:"name"`
}

// CommandSection represents a section in the config (simple, subcommands, or with flags)
type CommandSection struct {
	Commands    []string `toml:"commands"`
	Subcommands []string `toml:"subcommands"`
	Flags       []string `toml:"flags"`
}

// Config represents the full config.toml structure
type Config struct {
	Wrappers map[string]interface{} `toml:"wrappers"`
	Commands map[string]interface{} `toml:"commands"`
}

//go:embed config/config.toml
var defaultConfig []byte

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

// ensureConfigFiles creates the config directory and writes default config file if it doesn't exist.
func ensureConfigFiles(configDir string) error {
	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write default config.toml if it doesn't exist
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, defaultConfig, 0644); err != nil {
			return fmt.Errorf("failed to write config.toml: %w", err)
		}
	}

	return nil
}

// buildFlagPattern converts a flag specification to a regex pattern.
// "-f" becomes "(-f\s+)?"
// "-f <arg>" becomes "(-f\s*\S+\s+)?" (allows -f10 or -f 10)
// "<arg>" becomes "(\S+\s+)?" (positional argument)
// "" (empty) becomes "" (allows bare command)
func buildFlagPattern(flag string) string {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return ""
	}
	if flag == "<arg>" {
		return `(\S+\s+)?`
	}
	if strings.HasSuffix(flag, " <arg>") {
		flagName := strings.TrimSuffix(flag, " <arg>")
		// Allow optional space between flag and argument (e.g., -n10 or -n 10)
		return `(` + regexp.QuoteMeta(flagName) + `\s*\S+\s+)?`
	}
	return `(` + regexp.QuoteMeta(flag) + `\s+)?`
}

// buildSimplePattern creates a regex for a simple command (any args allowed).
// "pytest" becomes "^pytest\b"
func buildSimplePattern(cmd string) string {
	return `^` + regexp.QuoteMeta(cmd) + `\b`
}

// buildSubcommandPattern creates a regex for a command with subcommands and optional flags.
// cmd="git", subcommands=["diff","log"], flags=["-C <arg>"] becomes
// "^git\s+(-C\s+\S+\s+)?(diff|log)\b"
func buildSubcommandPattern(cmd string, subcommands []string, flags []string) string {
	var flagPatterns string
	for _, f := range flags {
		flagPatterns += buildFlagPattern(f)
	}

	// Escape subcommands and join with |
	escaped := make([]string, len(subcommands))
	for i, sub := range subcommands {
		escaped[i] = regexp.QuoteMeta(sub)
	}
	subPattern := strings.Join(escaped, "|")

	return `^` + regexp.QuoteMeta(cmd) + `\s+` + flagPatterns + `(` + subPattern + `)\b`
}

// buildWrapperPattern creates a regex for a wrapper command.
// For wrappers with flags, the pattern matches the command followed by flags.
// "timeout" with flags=["<arg>"] becomes "^timeout\s+(\S+\s+)?"
func buildWrapperPattern(cmd string, flags []string) string {
	var flagPatterns string
	for _, f := range flags {
		flagPatterns += buildFlagPattern(f)
	}
	if len(flags) > 0 {
		return `^` + regexp.QuoteMeta(cmd) + `\s+` + flagPatterns
	}
	return `^` + regexp.QuoteMeta(cmd) + `\s+`
}

// parseSection parses a config section and returns compiled patterns.
// isWrapper indicates if this is a wrapper section (affects pattern generation).
//
// Section types:
//   - simple: [*.simple] commands = ["cmd1", "cmd2"] - any arguments allowed
//   - command: [[*.command]] command = "cmd", flags = [...] - wrapper with flags
//   - subcommand: [[*.subcommand]] command = "cmd", subcommands = [...], flags = [...]
//   - regex: [[*.regex]] pattern = "^regex$", name = "desc" - raw regex
func parseSection(sectionData map[string]interface{}, isWrapper bool) ([]Pattern, error) {
	var patterns []Pattern

	for sectionType, value := range sectionData {
		switch sectionType {
		case "simple":
			// [*.simple] commands = ["cmd1", "cmd2"]
			sectionMap, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			cmds := toStringSlice(sectionMap["commands"])
			for _, cmd := range cmds {
				var pattern string
				var patternName string
				if isWrapper {
					pattern = buildWrapperPattern(cmd, nil)
					patternName = cmd
				} else {
					pattern = buildSimplePattern(cmd)
					patternName = "simple"
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
				}
				patterns = append(patterns, Pattern{Regex: re, Name: patternName})
			}

		case "command":
			// [[*.command]] command = "cmd", flags = [...]
			entries := toMapSlice(value)
			for _, entry := range entries {
				cmd, _ := entry["command"].(string)
				if cmd == "" {
					continue
				}
				flags := toStringSlice(entry["flags"])
				pattern := buildWrapperPattern(cmd, flags)
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
				}
				patterns = append(patterns, Pattern{Regex: re, Name: cmd})
			}

		case "subcommand":
			// [[*.subcommand]] command = "cmd", subcommands = [...], flags = [...]
			entries := toMapSlice(value)
			for _, entry := range entries {
				cmd, _ := entry["command"].(string)
				if cmd == "" {
					continue
				}
				subs := toStringSlice(entry["subcommands"])
				flags := toStringSlice(entry["flags"])
				if len(subs) == 0 {
					continue
				}
				pattern := buildSubcommandPattern(cmd, subs, flags)
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
				}
				patterns = append(patterns, Pattern{Regex: re, Name: cmd})
			}

		case "regex":
			// [[*.regex]] pattern = "^regex$", name = "desc"
			entries := toMapSlice(value)
			for _, entry := range entries {
				pattern, _ := entry["pattern"].(string)
				patternName, _ := entry["name"].(string)
				if pattern == "" {
					continue
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
				}
				patterns = append(patterns, Pattern{Regex: re, Name: patternName})
			}

		default:
			// Named sections like [commands.read-only] commands = [...]
			sectionMap, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			if cmdsRaw, ok := sectionMap["commands"]; ok {
				cmds := toStringSlice(cmdsRaw)
				for _, cmd := range cmds {
					pattern := buildSimplePattern(cmd)
					re, err := regexp.Compile(pattern)
					if err != nil {
						return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
					}
					patterns = append(patterns, Pattern{Regex: re, Name: sectionType})
				}
			}
		}
	}

	return patterns, nil
}

// toStringSlice converts an interface{} to []string
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// toMapSlice converts an interface{} to []map[string]interface{}
// Handles both []map[string]interface{} and []interface{} from TOML parsing
func toMapSlice(v interface{}) []map[string]interface{} {
	if v == nil {
		return nil
	}
	// Try direct type assertion first (TOML library sometimes returns this)
	if maps, ok := v.([]map[string]interface{}); ok {
		return maps
	}
	// Try []interface{} (more common from TOML parsing)
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

// loadConfig loads the config from TOML data and returns wrapper and command patterns.
func loadConfig(data []byte) (wrappers []Pattern, commands []Pattern, err error) {
	var raw map[string]map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	if wrappersSection, ok := raw["wrappers"]; ok {
		wrappers, err = parseSection(wrappersSection, true)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse wrappers: %w", err)
		}
	}

	if commandsSection, ok := raw["commands"]; ok {
		commands, err = parseSection(commandsSection, false)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse commands: %w", err)
		}
	}

	return wrappers, commands, nil
}

// loadEmbeddedDefaults loads patterns from the embedded default config file.
func loadEmbeddedDefaults() {
	wrapperPatterns, safeCommands, _ = loadConfig(defaultConfig)
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

	// Load config
	configPath := filepath.Join(configDir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to read config.toml: %w", err)
	}

	wrapperPatterns, safeCommands, err = loadConfig(configData)
	if err != nil {
		loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to load config: %w", err)
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
