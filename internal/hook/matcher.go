package hook

import (
	"strings"

	"github.com/dgerlanc/mmi/internal/patterns"
)

// CheckSafe checks if a command matches a safe pattern and returns details.
// Iterates through safeCommands in order and returns on first match.
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

// CheckDeny checks if a command matches a deny pattern and returns details.
// Iterates through denyPatterns in order and returns on first match.
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
