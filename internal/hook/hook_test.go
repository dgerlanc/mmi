package hook

import "testing"

func TestContainsDangerousPattern(t *testing.T) {
	tests := []struct {
		name      string
		cmd       string
		dangerous bool
	}{
		// Basic dangerous patterns should be rejected
		{
			name:      "command substitution with $()",
			cmd:       `echo $(whoami)`,
			dangerous: true,
		},
		{
			name:      "command substitution with backticks",
			cmd:       "echo `whoami`",
			dangerous: true,
		},
		{
			name:      "nested command substitution",
			cmd:       `echo $(echo $(whoami))`,
			dangerous: true,
		},

		// Safe commands without dangerous patterns
		{
			name:      "simple command",
			cmd:       `echo hello`,
			dangerous: false,
		},
		{
			name:      "command with quotes",
			cmd:       `echo "hello world"`,
			dangerous: false,
		},

		// Quoted heredocs should allow backticks and $()
		{
			name: "single-quoted heredoc with backticks",
			cmd: `cat << 'EOF'
hello ` + "`world`" + `
EOF`,
			dangerous: false,
		},
		{
			name: "single-quoted heredoc with $()",
			cmd: `cat << 'EOF'
hello $(world)
EOF`,
			dangerous: false,
		},
		{
			name: "double-quoted heredoc with backticks",
			cmd: `cat << "EOF"
hello ` + "`world`" + `
EOF`,
			dangerous: false,
		},
		{
			name: "double-quoted heredoc with $()",
			cmd: `cat << "EOF"
hello $(world)
EOF`,
			dangerous: false,
		},

		// Unquoted heredocs should still reject dangerous patterns
		{
			name: "unquoted heredoc with backticks",
			cmd: `cat << EOF
hello ` + "`world`" + `
EOF`,
			dangerous: true,
		},
		{
			name: "unquoted heredoc with $()",
			cmd: `cat << EOF
hello $(world)
EOF`,
			dangerous: true,
		},

		// Mixed: dangerous pattern outside heredoc
		{
			name: "dangerous pattern before quoted heredoc",
			cmd: `echo $(whoami) && cat << 'EOF'
safe content
EOF`,
			dangerous: true,
		},
		{
			name: "dangerous pattern after quoted heredoc",
			cmd: `cat << 'EOF'
safe content
EOF
echo $(whoami)`,
			dangerous: true,
		},

		// Real-world use case: Go code with backticks in heredoc
		{
			name: "go code in quoted heredoc",
			cmd: `cat > /tmp/test.go << 'EOF'
package main
var s = ` + "`hello`" + `
EOF`,
			dangerous: false,
		},

		// <<- operator (strip leading tabs)
		{
			name: "dash heredoc quoted with backticks",
			cmd: `cat <<- 'EOF'
	hello ` + "`world`" + `
	EOF`,
			dangerous: false,
		},
		{
			name: "dash heredoc unquoted with backticks",
			cmd: `cat <<- EOF
	hello ` + "`world`" + `
	EOF`,
			dangerous: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsDangerousPattern(tt.cmd)
			if result != tt.dangerous {
				t.Errorf("containsDangerousPattern(%q) = %v, want %v", tt.cmd, result, tt.dangerous)
			}
		})
	}
}

func TestFindQuotedHeredocRanges(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		wantRanges int
	}{
		{
			name:       "no heredoc",
			cmd:        `echo hello`,
			wantRanges: 0,
		},
		{
			name: "single quoted heredoc",
			cmd: `cat << 'EOF'
content
EOF`,
			wantRanges: 1,
		},
		{
			name: "double quoted heredoc",
			cmd: `cat << "EOF"
content
EOF`,
			wantRanges: 1,
		},
		{
			name: "unquoted heredoc",
			cmd: `cat << EOF
content
EOF`,
			wantRanges: 0,
		},
		{
			name: "multiple quoted heredocs",
			cmd: `cat << 'EOF1'
content1
EOF1
cat << 'EOF2'
content2
EOF2`,
			wantRanges: 2,
		},
		{
			name: "mixed quoted and unquoted heredocs",
			cmd: `cat << 'EOF1'
content1
EOF1
cat << EOF2
content2
EOF2`,
			wantRanges: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranges := findQuotedHeredocRanges(tt.cmd)
			if len(ranges) != tt.wantRanges {
				t.Errorf("findQuotedHeredocRanges(%q) returned %d ranges, want %d", tt.cmd, len(ranges), tt.wantRanges)
			}
		})
	}
}
