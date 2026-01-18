// Package patterns provides functions for building regex patterns
// to match shell commands in a structured way.
package patterns

import (
	"regexp"
	"strings"
)

// Pattern holds a compiled regex and its description.
type Pattern struct {
	Regex   *regexp.Regexp
	Name    string
	Type    string // simple, subcommand, command, regex
	Pattern string // original pattern string
}

// BuildFlagPattern converts a flag specification to a regex pattern.
// "-f" becomes "(-f\s+)?"
// "-f <arg>" becomes "(-f\s*\S+\s+)?" (allows -f10 or -f 10)
// "<arg>" becomes "(\S+\s+)?" (positional argument)
// "" (empty) becomes "" (allows bare command)
func BuildFlagPattern(flag string) string {
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

// BuildSimplePattern creates a regex for a simple command (any args allowed).
// "pytest" becomes "^pytest\b"
func BuildSimplePattern(cmd string) string {
	return `^` + regexp.QuoteMeta(cmd) + `\b`
}

// BuildSubcommandPattern creates a regex for a command with subcommands and optional flags.
// cmd="git", subcommands=["diff","log"], flags=["-C <arg>"] becomes
// "^git\s+(-C\s+\S+\s+)?(diff|log)\b"
func BuildSubcommandPattern(cmd string, subcommands []string, flags []string) string {
	var flagPatterns string
	for _, f := range flags {
		flagPatterns += BuildFlagPattern(f)
	}

	// Escape subcommands and join with |
	escaped := make([]string, len(subcommands))
	for i, sub := range subcommands {
		escaped[i] = regexp.QuoteMeta(sub)
	}
	subPattern := strings.Join(escaped, "|")

	return `^` + regexp.QuoteMeta(cmd) + `\s+` + flagPatterns + `(` + subPattern + `)\b`
}

// BuildWrapperPattern creates a regex for a wrapper command.
// For wrappers with flags, the pattern matches the command followed by flags.
// "timeout" with flags=["<arg>"] becomes "^timeout\s+(\S+\s+)?"
func BuildWrapperPattern(cmd string, flags []string) string {
	var flagPatterns string
	for _, f := range flags {
		flagPatterns += BuildFlagPattern(f)
	}
	if len(flags) > 0 {
		return `^` + regexp.QuoteMeta(cmd) + `\s+` + flagPatterns
	}
	return `^` + regexp.QuoteMeta(cmd) + `\s+`
}

// Compile compiles a pattern string into a Pattern with the given name.
// Returns an error if the pattern is invalid.
func Compile(pattern, name string) (Pattern, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Pattern{}, err
	}
	return Pattern{Regex: re, Name: name}, nil
}

// MustCompile is like Compile but panics if the pattern is invalid.
func MustCompile(pattern, name string) Pattern {
	p, err := Compile(pattern, name)
	if err != nil {
		panic(err)
	}
	return p
}
