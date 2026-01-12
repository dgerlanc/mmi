package patterns

import (
	"regexp"
	"testing"
)

func TestBuildFlagPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"positional arg", "<arg>", `(\S+\s+)?`},
		{"simple flag", "-f", `(-f\s+)?`},
		{"flag with arg", "-f <arg>", `(-f\s*\S+\s+)?`},
		{"long flag with arg", "-C <arg>", `(-C\s*\S+\s+)?`},
		{"long name flag", "--verbose", `(--verbose\s+)?`},
		{"long name with arg", "--config <arg>", `(--config\s*\S+\s+)?`},
		{"whitespace trimming", "  -f  ", `(-f\s+)?`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildFlagPattern(tt.input)
			if got != tt.expected {
				t.Errorf("BuildFlagPattern(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildFlagPattern_Regex(t *testing.T) {
	// Test that the generated patterns actually work as regex
	tests := []struct {
		name    string
		flag    string
		input   string
		matches bool
	}{
		{"empty allows anything", "", "anything", true},
		{"positional matches word", "<arg>", "value ", true},
		{"positional matches empty", "<arg>", "", true},
		{"simple flag matches", "-f", "-f ", true},
		{"simple flag matches optional", "-f", "-f", true}, // optional pattern matches empty suffix
		{"flag with arg compact", "-n <arg>", "-n10 ", true},
		{"flag with arg spaced", "-n <arg>", "-n 10 ", true},
		{"flag with arg optional", "-n <arg>", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := BuildFlagPattern(tt.flag)
			if pattern == "" {
				return // Empty pattern, skip regex test
			}
			re := regexp.MustCompile("^" + pattern)
			got := re.MatchString(tt.input)
			if got != tt.matches {
				t.Errorf("Pattern %q matching %q = %v, want %v", pattern, tt.input, got, tt.matches)
			}
		})
	}
}

func TestBuildSimplePattern(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected string
	}{
		{"simple command", "pytest", `^pytest\b`},
		{"command with hyphen", "my-cmd", `^my-cmd\b`}, // hyphen not escaped (only special in char classes)
		{"single char", "a", `^a\b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSimplePattern(tt.cmd)
			if got != tt.expected {
				t.Errorf("BuildSimplePattern(%q) = %q, want %q", tt.cmd, got, tt.expected)
			}
		})
	}
}

func TestBuildSimplePattern_Regex(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		input   string
		matches bool
	}{
		{"exact match", "pytest", "pytest", true},
		{"with args", "pytest", "pytest -v tests/", true},
		{"prefix only", "pytest", "pytester", false},
		{"word boundary works", "python", "python3", false},
		{"at start only", "pytest", "foo pytest", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := BuildSimplePattern(tt.cmd)
			re := regexp.MustCompile(pattern)
			got := re.MatchString(tt.input)
			if got != tt.matches {
				t.Errorf("Pattern %q matching %q = %v, want %v", pattern, tt.input, got, tt.matches)
			}
		})
	}
}

func TestBuildSubcommandPattern(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		subcommands []string
		flags       []string
		expected    string
	}{
		{
			name:        "simple subcommands",
			cmd:         "git",
			subcommands: []string{"diff", "log"},
			flags:       nil,
			expected:    `^git\s+(diff|log)\b`,
		},
		{
			name:        "single subcommand",
			cmd:         "git",
			subcommands: []string{"status"},
			flags:       nil,
			expected:    `^git\s+(status)\b`,
		},
		{
			name:        "with flag",
			cmd:         "git",
			subcommands: []string{"diff"},
			flags:       []string{"-C <arg>"},
			expected:    `^git\s+(-C\s*\S+\s+)?(diff)\b`,
		},
		{
			name:        "multiple flags",
			cmd:         "git",
			subcommands: []string{"log"},
			flags:       []string{"-C <arg>", "-n <arg>"},
			expected:    `^git\s+(-C\s*\S+\s+)?(-n\s*\S+\s+)?(log)\b`,
		},
		{
			name:        "special chars in subcommand",
			cmd:         "npm",
			subcommands: []string{"run-script"},
			flags:       nil,
			expected:    `^npm\s+(run-script)\b`, // hyphen not escaped (only special in char classes)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSubcommandPattern(tt.cmd, tt.subcommands, tt.flags)
			if got != tt.expected {
				t.Errorf("BuildSubcommandPattern() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildSubcommandPattern_Regex(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		subcommands []string
		flags       []string
		input       string
		matches     bool
	}{
		{"matches subcommand", "git", []string{"diff", "log"}, nil, "git diff", true},
		{"matches other subcommand", "git", []string{"diff", "log"}, nil, "git log", true},
		{"rejects unknown subcommand", "git", []string{"diff", "log"}, nil, "git push", false},
		{"with flag before subcommand", "git", []string{"diff"}, []string{"-C <arg>"}, "git -C /path diff", true},
		{"without flag", "git", []string{"diff"}, []string{"-C <arg>"}, "git diff", true},
		{"flag compact notation", "git", []string{"log"}, []string{"-n <arg>"}, "git -n10 log", true},
		{"subcommand with args", "git", []string{"diff"}, nil, "git diff HEAD~1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := BuildSubcommandPattern(tt.cmd, tt.subcommands, tt.flags)
			re := regexp.MustCompile(pattern)
			got := re.MatchString(tt.input)
			if got != tt.matches {
				t.Errorf("Pattern %q matching %q = %v, want %v", pattern, tt.input, got, tt.matches)
			}
		})
	}
}

func TestBuildWrapperPattern(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		flags    []string
		expected string
	}{
		{
			name:     "no flags",
			cmd:      "env",
			flags:    nil,
			expected: `^env\s+`,
		},
		{
			name:     "with positional arg",
			cmd:      "timeout",
			flags:    []string{"<arg>"},
			expected: `^timeout\s+(\S+\s+)?`,
		},
		{
			name:     "with flag arg",
			cmd:      "nice",
			flags:    []string{"-n <arg>"},
			expected: `^nice\s+(-n\s*\S+\s+)?`,
		},
		{
			name:     "multiple flag options",
			cmd:      "nice",
			flags:    []string{"-n <arg>", ""},
			expected: `^nice\s+(-n\s*\S+\s+)?`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildWrapperPattern(tt.cmd, tt.flags)
			if got != tt.expected {
				t.Errorf("BuildWrapperPattern() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildWrapperPattern_Regex(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		flags   []string
		input   string
		matches bool
	}{
		{"env wrapper", "env", nil, "env ", true},
		{"env with command", "env", nil, "env pytest", true},
		{"env no space", "env", nil, "env", false},
		{"timeout with arg", "timeout", []string{"<arg>"}, "timeout 30 ", true},
		{"timeout compact", "timeout", []string{"<arg>"}, "timeout 30 pytest", true},
		{"nice with flag", "nice", []string{"-n <arg>"}, "nice -n 10 ", true},
		{"nice compact flag", "nice", []string{"-n <arg>"}, "nice -n10 ", true},
		{"nice without flag", "nice", []string{"-n <arg>"}, "nice ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := BuildWrapperPattern(tt.cmd, tt.flags)
			re := regexp.MustCompile(pattern)
			got := re.MatchString(tt.input)
			if got != tt.matches {
				t.Errorf("Pattern %q matching %q = %v, want %v", pattern, tt.input, got, tt.matches)
			}
		})
	}
}

func TestCompile(t *testing.T) {
	t.Run("valid pattern", func(t *testing.T) {
		p, err := Compile(`^test\b`, "test command")
		if err != nil {
			t.Errorf("Compile() error = %v", err)
		}
		if p.Name != "test command" {
			t.Errorf("Pattern.Name = %q, want %q", p.Name, "test command")
		}
		if p.Regex == nil {
			t.Error("Pattern.Regex is nil")
		}
		if !p.Regex.MatchString("test arg") {
			t.Error("Pattern should match 'test arg'")
		}
	})

	t.Run("invalid pattern", func(t *testing.T) {
		_, err := Compile(`[invalid`, "bad")
		if err == nil {
			t.Error("Compile() should return error for invalid pattern")
		}
	})
}

func TestMustCompile(t *testing.T) {
	t.Run("valid pattern", func(t *testing.T) {
		p := MustCompile(`^test\b`, "test")
		if p.Regex == nil {
			t.Error("Pattern.Regex is nil")
		}
	})

	t.Run("invalid pattern panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustCompile() should panic for invalid pattern")
			}
		}()
		MustCompile(`[invalid`, "bad")
	})
}

func TestPattern_Match(t *testing.T) {
	p := MustCompile(`^git\s+(status|diff)\b`, "git")

	tests := []struct {
		input   string
		matches bool
	}{
		{"git status", true},
		{"git diff", true},
		{"git diff HEAD", true},
		{"git push", false},
		{"git", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := p.Regex.MatchString(tt.input)
			if got != tt.matches {
				t.Errorf("Pattern matching %q = %v, want %v", tt.input, got, tt.matches)
			}
		})
	}
}
