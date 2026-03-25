package cmdpath

import "strings"

func init() {
	register(CommandDescriptor{
		Name:           "rm",
		ExtractTargets: extractRmTargets,
	})
	register(CommandDescriptor{
		Name:           "mv",
		ExtractTargets: extractMvTargets,
	})
	register(CommandDescriptor{
		Name:           "chmod",
		ExtractTargets: extractChmodTargets,
	})
	register(CommandDescriptor{
		Name:           "chown",
		ExtractTargets: extractChownTargets,
	})
}

// isUnresolvable returns true if the arg contains shell variable references
// or other syntax that prevents static path resolution.
func isUnresolvable(arg string) bool {
	return strings.ContainsAny(arg, "$`")
}

// isFlag returns true if the arg looks like a command flag.
func isFlag(arg string) bool {
	return strings.HasPrefix(arg, "-")
}

// isFlagWithArg returns true if the flag expects a following argument.
func isFlagWithArg(command, flag string) bool {
	switch command {
	case "mv":
		return flag == "-t" || flag == "--target-directory" ||
			flag == "-S" || flag == "--suffix"
	case "rm":
		return false
	}
	return false
}

// extractSimpleTargets extracts non-flag args as targets, handling -- and variables.
// Used by commands where all non-flag positional args are filesystem targets (rm, mv).
func extractSimpleTargets(command string, args []string) (targets []string, unresolved []string) {
	pastDoubleDash := false
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !pastDoubleDash && arg == "--" {
			pastDoubleDash = true
			continue
		}

		if !pastDoubleDash && isFlag(arg) {
			if !strings.Contains(arg, "=") && isFlagWithArg(command, arg) && i+1 < len(args) {
				i++
				nextArg := args[i]
				if isUnresolvable(nextArg) {
					unresolved = append(unresolved, nextArg)
				} else {
					targets = append(targets, nextArg)
				}
			}
			continue
		}

		if isUnresolvable(arg) {
			unresolved = append(unresolved, arg)
		} else {
			targets = append(targets, arg)
		}
	}
	return targets, unresolved
}

func extractRmTargets(args []string) ([]string, []string) {
	return extractSimpleTargets("rm", args)
}

func extractMvTargets(args []string) ([]string, []string) {
	return extractSimpleTargets("mv", args)
}

// Stub implementations for chmod/chown — filled in next task
func extractChmodTargets(args []string) ([]string, []string) { return nil, nil }
func extractChownTargets(args []string) ([]string, []string) { return nil, nil }
