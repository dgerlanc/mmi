package hook

import (
	"strings"

	"github.com/dgerlanc/mmi/internal/patterns"
)

// SafeResult contains detailed information about a safe pattern match.
type SafeResult struct {
	Matched bool
	Name    string
	Type    string // simple, subcommand, regex, command
	Pattern string
}

// CheckSafe checks if a command matches a safe pattern and returns details.
func CheckSafe(cmd string, safeCommands []patterns.Pattern) SafeResult {
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

// CheckDeny checks if a command matches a deny pattern and returns details.
func CheckDeny(cmd string, denyPatterns []patterns.Pattern) DenyResult {
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
