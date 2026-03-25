package cmdpath

import "regexp"

// chmodModePattern matches numeric modes (e.g., 755, 0644) and symbolic modes (e.g., u+x, go-rwx).
// Note: does not match compound symbolic modes like "u+x,g-w". Those fall through to
// path checking and will be rejected (fail closed), prompting the user for approval.
var chmodModePattern = regexp.MustCompile(`^[0-7]{3,4}$|^[ugoa]*[+-=][rwxXst]+$`)

// chownOwnerPattern matches user, user:group, :group, and user: patterns.
// Note: this pattern also matches strings that look like filenames (e.g., "foo.txt").
// In practice, chown always takes an owner as the first positional arg, so
// misidentification only occurs with malformed commands that would fail at runtime anyway.
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
