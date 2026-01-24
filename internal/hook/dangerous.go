package hook

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// dangerousPattern matches command substitution syntax
var dangerousPattern = regexp.MustCompile(`\$\(|` + "`")

// byteRange represents a range of bytes in a string
type byteRange struct {
	start, end int
}

// findQuotedHeredocRanges parses a command and returns byte ranges of heredoc content
// where the delimiter is quoted (single or double quotes). Quoted heredocs don't perform
// shell expansion, so backticks and $() inside them are literal text, not command substitution.
func findQuotedHeredocRanges(cmd string) []byteRange {
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil
	}

	var ranges []byteRange
	syntax.Walk(prog, func(node syntax.Node) bool {
		redir, ok := node.(*syntax.Redirect)
		if !ok {
			return true
		}

		// Check if this is a heredoc operator (<< or <<-)
		if redir.Op != syntax.Hdoc && redir.Op != syntax.DashHdoc {
			return true
		}

		// Check if the delimiter is quoted
		if redir.Word == nil || len(redir.Word.Parts) == 0 {
			return true
		}

		isQuoted := false
		for _, part := range redir.Word.Parts {
			switch part.(type) {
			case *syntax.SglQuoted, *syntax.DblQuoted:
				isQuoted = true
			}
		}

		// If quoted and has heredoc content, record the range
		if isQuoted && redir.Hdoc != nil {
			start := int(redir.Hdoc.Pos().Offset())
			end := int(redir.Hdoc.End().Offset())
			if start < end && start >= 0 && end <= len(cmd) {
				ranges = append(ranges, byteRange{start: start, end: end})
			}
		}

		return true
	})

	return ranges
}

// containsDangerousPattern checks if the command contains dangerous patterns ($( or backticks)
// while excluding content inside quoted heredocs where these characters are literal.
func containsDangerousPattern(cmd string) bool {
	excludeRanges := findQuotedHeredocRanges(cmd)

	// If no heredocs, do the simple check
	if len(excludeRanges) == 0 {
		return dangerousPattern.MatchString(cmd)
	}

	// Find all matches of the dangerous pattern
	matches := dangerousPattern.FindAllStringIndex(cmd, -1)
	if matches == nil {
		return false
	}

	// Check if any match is outside the excluded ranges
	for _, match := range matches {
		matchStart := match[0]
		inExcludedRange := false
		for _, r := range excludeRanges {
			if matchStart >= r.start && matchStart < r.end {
				inExcludedRange = true
				break
			}
		}
		if !inExcludedRange {
			return true
		}
	}

	return false
}
