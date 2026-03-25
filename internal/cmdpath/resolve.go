package cmdpath

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPathVariables expands $PROJECT and $PROJECT_ROOT in path expressions.
func ExpandPathVariables(paths []string, cwd, gitRoot string) []string {
	result := make([]string, len(paths))
	for i, p := range paths {
		switch p {
		case "$PROJECT":
			result[i] = cwd
		case "$PROJECT_ROOT":
			result[i] = gitRoot
		default:
			result[i] = p
		}
	}
	return result
}

// ExpandTilde expands ~ to $HOME. Returns the expanded path and whether it was resolvable.
// ~user paths are not resolvable and return (original, false).
func ExpandTilde(path string) (string, bool) {
	if !strings.HasPrefix(path, "~") {
		return path, true
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home := os.Getenv("HOME")
		if home == "" {
			return path, false
		}
		if path == "~" {
			return home, true
		}
		return filepath.Join(home, path[2:]), true
	}
	return path, false
}

// ResolveTargets resolves relative paths against cwd and cleans all paths.
func ResolveTargets(targets []string, cwd string) []string {
	result := make([]string, len(targets))
	for i, t := range targets {
		if filepath.IsAbs(t) {
			result[i] = filepath.Clean(t)
		} else {
			result[i] = filepath.Clean(filepath.Join(cwd, t))
		}
	}
	return result
}

// CheckPathPrefixes checks that every target is under at least one allowed prefix.
// Uses directory-boundary-aware prefix matching (not just string prefix).
func CheckPathPrefixes(targets []string, allowed []string) bool {
	for _, target := range targets {
		if !isUnderAnyPrefix(target, allowed) {
			return false
		}
	}
	return true
}

func isUnderAnyPrefix(target string, allowed []string) bool {
	cleanTarget := filepath.Clean(target)
	for _, prefix := range allowed {
		cleanPrefix := filepath.Clean(prefix)
		if cleanTarget == cleanPrefix {
			return true
		}
		if !strings.HasSuffix(cleanPrefix, string(filepath.Separator)) {
			cleanPrefix += string(filepath.Separator)
		}
		if strings.HasPrefix(cleanTarget, cleanPrefix) {
			return true
		}
	}
	return false
}
