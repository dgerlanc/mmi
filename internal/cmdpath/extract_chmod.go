package cmdpath

import "regexp"

// chmodModePattern matches numeric modes (e.g., 755, 0644) and symbolic modes (e.g., u+x, go-rwx).
var chmodModePattern = regexp.MustCompile(`^[0-7]{3,4}$|^[ugoa]*[+-=][rwxXst]+$`)

// chownOwnerPattern matches user, user:group, :group, and user: patterns.
var chownOwnerPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]*(:[a-zA-Z0-9._-]*)?$`)

func extractChmodTargets(args []string) ([]string, []string) {
	return extractWithFirstArgSkip(args, chmodModePattern)
}

func extractChownTargets(args []string) ([]string, []string) {
	return extractWithFirstArgSkip(args, chownOwnerPattern)
}

// extractWithFirstArgSkip extracts targets where the first non-flag positional arg
// is a special value (mode for chmod, owner for chown) that should be skipped.
func extractWithFirstArgSkip(args []string, skipPattern *regexp.Regexp) (targets []string, unresolved []string) {
	pastDoubleDash := false
	skippedFirst := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !pastDoubleDash && arg == "--" {
			pastDoubleDash = true
			continue
		}

		if !pastDoubleDash && isFlag(arg) {
			continue
		}

		// Skip the first positional arg if it matches the pattern (mode/owner)
		if !pastDoubleDash && !skippedFirst && skipPattern.MatchString(arg) {
			skippedFirst = true
			continue
		}
		skippedFirst = true

		if isUnresolvable(arg) {
			unresolved = append(unresolved, arg)
		} else {
			targets = append(targets, arg)
		}
	}
	return targets, unresolved
}
