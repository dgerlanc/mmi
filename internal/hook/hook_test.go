package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dgerlanc/mmi/internal/audit"
	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/patterns"
)

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

// Phase 2: Session and ToolUseID Tracking Tests

// setupTestAudit sets up a temp audit log and returns the path and cleanup function
func setupTestAudit(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "mmi-hook-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	logPath := filepath.Join(tmpDir, "audit.log")

	if err := audit.Init(logPath, false); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init audit: %v", err)
	}

	return logPath, func() {
		audit.Close()
		audit.Reset()
		os.RemoveAll(tmpDir)
	}
}

// readLastAuditEntry reads and parses the last entry from the audit log
func readLastAuditEntry(t *testing.T, logPath string) audit.Entry {
	audit.Close() // Ensure file is flushed

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) == 0 {
		t.Fatal("No audit entries found")
	}

	var entry audit.Entry
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("Failed to parse audit entry: %v", err)
	}
	return entry
}

func TestProcessWithResultPassesSessionID(t *testing.T) {
	config.Reset()
	defer config.Reset()

	// Set up config with a safe command
	configData := []byte(`
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	cfg, err := config.LoadConfig(configData)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	// We need to set the global config - this is a bit hacky
	// For now, let's just test the struct values are passed correctly

	_ = cfg // We'll use this in Phase 4 when we have better testability

	logPath, cleanup := setupTestAudit(t)
	defer cleanup()

	input := `{
		"session_id": "test-session-123",
		"tool_use_id": "test-tool-456",
		"cwd": "/test/dir",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.SessionID != "test-session-123" {
		t.Errorf("SessionID = %q, want %q", entry.SessionID, "test-session-123")
	}
}

func TestProcessWithResultPassesToolUseID(t *testing.T) {
	config.Reset()
	defer config.Reset()

	logPath, cleanup := setupTestAudit(t)
	defer cleanup()

	input := `{
		"session_id": "test-session-123",
		"tool_use_id": "test-tool-456",
		"cwd": "/test/dir",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.ToolUseID != "test-tool-456" {
		t.Errorf("ToolUseID = %q, want %q", entry.ToolUseID, "test-tool-456")
	}
}

func TestProcessWithResultPassesCwd(t *testing.T) {
	config.Reset()
	defer config.Reset()

	logPath, cleanup := setupTestAudit(t)
	defer cleanup()

	input := `{
		"session_id": "test-session-123",
		"tool_use_id": "test-tool-456",
		"cwd": "/test/dir",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.Cwd != "/test/dir" {
		t.Errorf("Cwd = %q, want %q", entry.Cwd, "/test/dir")
	}
}

func TestProcessWithResultPassesAllFields(t *testing.T) {
	config.Reset()
	defer config.Reset()

	logPath, cleanup := setupTestAudit(t)
	defer cleanup()

	input := `{
		"session_id": "sess-abc",
		"tool_use_id": "tool-xyz",
		"cwd": "/home/user/project",
		"tool_name": "Bash",
		"tool_input": {"command": "ls -la"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	if entry.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", entry.SessionID, "sess-abc")
	}
	if entry.ToolUseID != "tool-xyz" {
		t.Errorf("ToolUseID = %q, want %q", entry.ToolUseID, "tool-xyz")
	}
	if entry.Cwd != "/home/user/project" {
		t.Errorf("Cwd = %q, want %q", entry.Cwd, "/home/user/project")
	}
}

// Phase 3: Pattern Match Results Tests

func TestCheckSafeResultMatchedTrue(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "ls", patternType: "simple", pattern: `^ls\b`},
	})

	result := CheckSafe("ls -la", patterns)

	if !result.Matched {
		t.Error("Expected Matched=true for 'ls' command")
	}
	if result.Name != "ls" {
		t.Errorf("Name = %q, want %q", result.Name, "ls")
	}
	if result.Type != "simple" {
		t.Errorf("Type = %q, want %q", result.Type, "simple")
	}
	if result.Pattern != `^ls\b` {
		t.Errorf("Pattern = %q, want %q", result.Pattern, `^ls\b`)
	}
}

func TestCheckSafeResultMatchedFalse(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "ls", patternType: "simple", pattern: `^ls\b`},
	})

	result := CheckSafe("curl http://example.com", patterns)

	if result.Matched {
		t.Error("Expected Matched=false for unknown command")
	}
}

func TestCheckSafeResultSimpleType(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "pwd", patternType: "simple", pattern: `^pwd\b`},
	})

	result := CheckSafe("pwd", patterns)

	if result.Type != "simple" {
		t.Errorf("Type = %q, want %q", result.Type, "simple")
	}
}

func TestCheckSafeResultSubcommandType(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "git", patternType: "subcommand", pattern: `^git\s+(status|log)\b`},
	})

	result := CheckSafe("git status", patterns)

	if result.Type != "subcommand" {
		t.Errorf("Type = %q, want %q", result.Type, "subcommand")
	}
}

func TestCheckSafeResultRegexType(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "custom", patternType: "regex", pattern: `^mycommand\s+.*`},
	})

	result := CheckSafe("mycommand foo bar", patterns)

	if result.Type != "regex" {
		t.Errorf("Type = %q, want %q", result.Type, "regex")
	}
}

func TestCheckSafeResultCommandType(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "timeout", patternType: "command", pattern: `^timeout\s+\d+\s+`},
	})

	result := CheckSafe("timeout 10 ls", patterns)

	if result.Type != "command" {
		t.Errorf("Type = %q, want %q", result.Type, "command")
	}
}

func TestCheckDenyResultDeniedTrue(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "rm dangerous", patternType: "regex", pattern: `^rm\s+-rf\s+/`},
	})

	result := CheckDeny("rm -rf /", patterns)

	if !result.Denied {
		t.Error("Expected Denied=true for 'rm -rf /'")
	}
	if result.Name != "rm dangerous" {
		t.Errorf("Name = %q, want %q", result.Name, "rm dangerous")
	}
	if result.Pattern != `^rm\s+-rf\s+/` {
		t.Errorf("Pattern = %q, want %q", result.Pattern, `^rm\s+-rf\s+/`)
	}
}

func TestCheckDenyResultDeniedFalse(t *testing.T) {
	patterns := mustCompilePatterns(t, []patternDef{
		{name: "rm dangerous", patternType: "regex", pattern: `^rm\s+-rf\s+/`},
	})

	result := CheckDeny("ls -la", patterns)

	if result.Denied {
		t.Error("Expected Denied=false for 'ls -la'")
	}
}

// Helper types and functions for tests

type patternDef struct {
	name        string
	patternType string
	pattern     string
}

func mustCompilePatterns(t *testing.T, defs []patternDef) []patterns.Pattern {
	t.Helper()
	result := make([]patterns.Pattern, len(defs))
	for i, def := range defs {
		re, err := regexp.Compile(def.pattern)
		if err != nil {
			t.Fatalf("Failed to compile pattern %q: %v", def.pattern, err)
		}
		result[i] = patterns.Pattern{
			Regex:   re,
			Name:    def.name,
			Type:    def.patternType,
			Pattern: def.pattern,
		}
	}
	return result
}

// Phase 4: Hook Integration Tests

// setupTestConfig creates a test configuration with specified patterns
func setupTestConfig(t *testing.T, configTOML string) func() {
	t.Helper()
	config.Reset()

	// Create a temp config directory
	tmpDir, err := os.MkdirTemp("", "mmi-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set MMI_CONFIG env var
	origConfig := os.Getenv("MMI_CONFIG")
	os.Setenv("MMI_CONFIG", tmpDir)

	// Write the config
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configTOML), 0644); err != nil {
		os.RemoveAll(tmpDir)
		os.Setenv("MMI_CONFIG", origConfig)
		t.Fatalf("Failed to write config: %v", err)
	}

	// Initialize config
	if err := config.Init(); err != nil {
		os.RemoveAll(tmpDir)
		os.Setenv("MMI_CONFIG", origConfig)
		t.Fatalf("Failed to init config: %v", err)
	}

	return func() {
		config.Reset()
		os.RemoveAll(tmpDir)
		os.Setenv("MMI_CONFIG", origConfig)
	}
}

func TestSegmentPopulationSingleCommand(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if len(entry.Segments) != 1 {
		t.Errorf("Expected 1 segment, got %d", len(entry.Segments))
	}
}

func TestSegmentPopulationChainedCommands(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls", "pwd"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls && pwd"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if len(entry.Segments) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(entry.Segments))
	}
}

func TestSegmentOrderMatchesCommandOrder(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls", "pwd", "whoami"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls && pwd && whoami"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if len(entry.Segments) != 3 {
		t.Fatalf("Expected 3 segments, got %d", len(entry.Segments))
	}
	if entry.Segments[0].Command != "ls" {
		t.Errorf("First segment command = %q, want %q", entry.Segments[0].Command, "ls")
	}
	if entry.Segments[1].Command != "pwd" {
		t.Errorf("Second segment command = %q, want %q", entry.Segments[1].Command, "pwd")
	}
	if entry.Segments[2].Command != "whoami" {
		t.Errorf("Third segment command = %q, want %q", entry.Segments[2].Command, "whoami")
	}
}

func TestApprovedSegmentMatchType(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.subcommand]]
command = "git"
subcommands = ["status", "log"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "git status"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}
	seg := entry.Segments[0]
	if seg.Match == nil {
		t.Fatal("Expected Match to be set for approved segment")
	}
	if seg.Match.Type != "subcommand" {
		t.Errorf("Match.Type = %q, want %q", seg.Match.Type, "subcommand")
	}
	if seg.Match.Name != "git" {
		t.Errorf("Match.Name = %q, want %q", seg.Match.Name, "git")
	}
}

func TestApprovedSegmentWithSingleWrapper(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[wrappers]
[[wrappers.simple]]
commands = ["sudo"]

[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "sudo ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}
	seg := entry.Segments[0]
	if len(seg.Wrappers) != 1 {
		t.Errorf("Expected 1 wrapper, got %d", len(seg.Wrappers))
	}
	if len(seg.Wrappers) > 0 && seg.Wrappers[0] != "sudo" {
		t.Errorf("Wrapper = %q, want %q", seg.Wrappers[0], "sudo")
	}
}

func TestApprovedSegmentWithNoWrappers(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}
	seg := entry.Segments[0]
	if len(seg.Wrappers) != 0 {
		t.Errorf("Expected no wrappers, got %v", seg.Wrappers)
	}
}

func TestRejectedSegmentCommandSubstitution(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls $(whoami)"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.Approved {
		t.Error("Expected command to be rejected")
	}
	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}
	seg := entry.Segments[0]
	if seg.Rejection == nil {
		t.Fatal("Expected Rejection to be set")
	}
	if seg.Rejection.Code != audit.CodeCommandSubstitution {
		t.Errorf("Rejection.Code = %q, want %q", seg.Rejection.Code, audit.CodeCommandSubstitution)
	}
}

func TestRejectedSegmentUnparseable(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "echo 'unclosed"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.Approved {
		t.Error("Expected command to be rejected")
	}
	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}
	seg := entry.Segments[0]
	if seg.Rejection == nil {
		t.Fatal("Expected Rejection to be set")
	}
	if seg.Rejection.Code != audit.CodeUnparseable {
		t.Errorf("Rejection.Code = %q, want %q", seg.Rejection.Code, audit.CodeUnparseable)
	}
}

func TestRejectedSegmentDenyMatch(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls"]

[deny]
[[deny.regex]]
name = "dangerous rm"
pattern = "^rm\\s+-rf\\s+/"
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf /"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.Approved {
		t.Error("Expected command to be rejected")
	}
	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}
	seg := entry.Segments[0]
	if seg.Rejection == nil {
		t.Fatal("Expected Rejection to be set")
	}
	if seg.Rejection.Code != audit.CodeDenyMatch {
		t.Errorf("Rejection.Code = %q, want %q", seg.Rejection.Code, audit.CodeDenyMatch)
	}
	if seg.Rejection.Name != "dangerous rm" {
		t.Errorf("Rejection.Name = %q, want %q", seg.Rejection.Name, "dangerous rm")
	}
}

func TestRejectedSegmentNoMatch(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "curl http://example.com"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.Approved {
		t.Error("Expected command to be rejected")
	}
	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}
	seg := entry.Segments[0]
	if seg.Rejection == nil {
		t.Fatal("Expected Rejection to be set")
	}
	if seg.Rejection.Code != audit.CodeNoMatch {
		t.Errorf("Rejection.Code = %q, want %q", seg.Rejection.Code, audit.CodeNoMatch)
	}
}

func TestEntryApprovedWhenAllSegmentsApproved(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls", "pwd"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls && pwd"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if !entry.Approved {
		t.Error("Expected Entry.Approved=true when all segments are approved")
	}
	for i, seg := range entry.Segments {
		if !seg.Approved {
			t.Errorf("Segment[%d].Approved = false, want true", i)
		}
	}
}

func TestEntryRejectedWhenAnySegmentRejected(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls && curl http://example.com"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.Approved {
		t.Error("Expected Entry.Approved=false when any segment is rejected")
	}
}

func TestEntryDurationMsPopulated(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	if entry.DurationMs <= 0 {
		t.Errorf("Expected DurationMs > 0, got %v", entry.DurationMs)
	}
}

// Phase 5: All Segments Evaluation Tests
// These tests verify that ALL segments in a piped/chained command are evaluated
// and logged, even when one segment is rejected.

func TestAllSegmentsEvaluatedInPipe(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["echo"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	// First segment (echo 'sudo rm -rf /') is safe
	// Second segment (./mmi --dry-run) is not in safe list
	// Both should be logged
	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "echo 'test' | cat"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)
	// Both segments should be logged even though cat is not in safe list
	if len(entry.Segments) != 2 {
		t.Errorf("Expected 2 segments in audit log, got %d", len(entry.Segments))
	}
}

func TestMultipleRejectedSegmentsAllCaptured(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "echo"
commands = ["echo"]

[deny]
[[deny.regex]]
name = "dangerous rm"
pattern = "^rm\\s+-rf\\s+/"
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	// First segment: rm -rf / (denied)
	// Second segment: curl (no match)
	// Both should be logged with their respective rejection reasons
	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf / && curl http://evil.com"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	if len(entry.Segments) != 2 {
		t.Fatalf("Expected 2 segments in audit log, got %d", len(entry.Segments))
	}

	// First segment should be denied
	if entry.Segments[0].Rejection == nil {
		t.Fatal("Expected first segment to have rejection")
	}
	if entry.Segments[0].Rejection.Code != audit.CodeDenyMatch {
		t.Errorf("First segment Rejection.Code = %q, want %q", entry.Segments[0].Rejection.Code, audit.CodeDenyMatch)
	}

	// Second segment should also be evaluated (no match)
	if entry.Segments[1].Rejection == nil {
		t.Fatal("Expected second segment to have rejection")
	}
	if entry.Segments[1].Rejection.Code != audit.CodeNoMatch {
		t.Errorf("Second segment Rejection.Code = %q, want %q", entry.Segments[1].Rejection.Code, audit.CodeNoMatch)
	}
}

func TestMixedApprovedRejectedSegments(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls", "pwd"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	// First segment: ls (approved)
	// Second segment: curl (rejected - no match)
	// Third segment: pwd (would be approved, but should still be evaluated and logged)
	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls && curl http://example.com && pwd"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	if len(entry.Segments) != 3 {
		t.Fatalf("Expected 3 segments in audit log, got %d", len(entry.Segments))
	}

	// Overall should be rejected
	if entry.Approved {
		t.Error("Expected overall command to be rejected")
	}

	// First segment (ls) should be approved
	if !entry.Segments[0].Approved {
		t.Error("Expected first segment (ls) to be approved")
	}
	if entry.Segments[0].Match == nil {
		t.Error("Expected first segment to have match info")
	}

	// Second segment (curl) should be rejected
	if entry.Segments[1].Approved {
		t.Error("Expected second segment (curl) to be rejected")
	}
	if entry.Segments[1].Rejection == nil {
		t.Fatal("Expected second segment to have rejection")
	}
	if entry.Segments[1].Rejection.Code != audit.CodeNoMatch {
		t.Errorf("Second segment Rejection.Code = %q, want %q", entry.Segments[1].Rejection.Code, audit.CodeNoMatch)
	}

	// Third segment (pwd) should still be evaluated and approved
	if !entry.Segments[2].Approved {
		t.Error("Expected third segment (pwd) to be approved")
	}
	if entry.Segments[2].Match == nil {
		t.Error("Expected third segment to have match info")
	}
}

func TestDenyMatchStillEvaluatesSubsequentSegments(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls", "pwd", "echo"]

[deny]
[[deny.regex]]
name = "dangerous rm"
pattern = "^rm\\s+-rf\\s+/"
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	// First segment: rm -rf / (denied)
	// Second segment: ls (would be approved)
	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf / && ls"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	if len(entry.Segments) != 2 {
		t.Fatalf("Expected 2 segments in audit log, got %d", len(entry.Segments))
	}

	// First segment should be deny match
	if entry.Segments[0].Rejection == nil || entry.Segments[0].Rejection.Code != audit.CodeDenyMatch {
		t.Error("Expected first segment to be DENY_MATCH")
	}

	// Second segment should be evaluated and approved
	if !entry.Segments[1].Approved {
		t.Error("Expected second segment (ls) to be approved")
	}
}

func TestCommandSubstitutionPerSegment(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls", "echo"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	// First segment: ls (approved)
	// Second segment: echo $(whoami) (command substitution - rejected)
	// Third segment: ls (approved)
	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls && echo $(whoami) && ls -la"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	// All three segments should be evaluated
	if len(entry.Segments) != 3 {
		t.Fatalf("Expected 3 segments in audit log, got %d", len(entry.Segments))
	}

	// Overall should be rejected
	if entry.Approved {
		t.Error("Expected overall command to be rejected")
	}

	// First segment (ls) should be approved
	if !entry.Segments[0].Approved {
		t.Error("Expected first segment (ls) to be approved")
	}

	// Second segment (echo $(whoami)) should be rejected for command substitution
	if entry.Segments[1].Approved {
		t.Error("Expected second segment to be rejected")
	}
	if entry.Segments[1].Rejection == nil {
		t.Fatal("Expected second segment to have rejection")
	}
	if entry.Segments[1].Rejection.Code != audit.CodeCommandSubstitution {
		t.Errorf("Second segment Rejection.Code = %q, want %q", entry.Segments[1].Rejection.Code, audit.CodeCommandSubstitution)
	}

	// Third segment (ls -la) should still be approved
	if !entry.Segments[2].Approved {
		t.Error("Expected third segment (ls -la) to be approved")
	}
}

func TestCommandSubstitutionOnlyInOneSegment(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "basic"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	// Only the segment with command substitution should be rejected
	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls $(pwd)"}
	}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	if len(entry.Segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(entry.Segments))
	}

	if entry.Segments[0].Rejection == nil {
		t.Fatal("Expected segment to have rejection")
	}
	if entry.Segments[0].Rejection.Code != audit.CodeCommandSubstitution {
		t.Errorf("Rejection.Code = %q, want %q", entry.Segments[0].Rejection.Code, audit.CodeCommandSubstitution)
	}
}

// Phase 6: Raw Input Capture Tests

func TestProcessWithResultCapturesRawInput(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	rawInput := `{"session_id":"sess-raw","tool_use_id":"tool-raw","cwd":"/test","tool_name":"Bash","tool_input":{"command":"ls"}}`

	ProcessWithResult(strings.NewReader(rawInput))

	entry := readLastAuditEntry(t, logPath)

	if entry.Input != rawInput {
		t.Errorf("Input = %q, want %q", entry.Input, rawInput)
	}
}

func TestProcessWithResultCapturesRawInputOnRejection(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	rawInput := `{"session_id":"sess-raw","tool_use_id":"tool-raw","cwd":"/test","tool_name":"Bash","tool_input":{"command":"curl http://example.com"}}`

	ProcessWithResult(strings.NewReader(rawInput))

	entry := readLastAuditEntry(t, logPath)

	if entry.Input != rawInput {
		t.Errorf("Input = %q, want %q", entry.Input, rawInput)
	}
}

func TestProcessWithResultCapturesRawInputOnUnparseable(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	rawInput := `{"session_id":"sess-raw","tool_use_id":"tool-raw","cwd":"/test","tool_name":"Bash","tool_input":{"command":"echo 'unclosed"}}`

	ProcessWithResult(strings.NewReader(rawInput))

	entry := readLastAuditEntry(t, logPath)

	if entry.Input != rawInput {
		t.Errorf("Input = %q, want %q", entry.Input, rawInput)
	}
}

func TestProcessWithResultCapturesOutputOnApproval(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{"session_id":"sess-1","tool_use_id":"tool-1","cwd":"/test","tool_name":"Bash","tool_input":{"command":"ls"}}`

	result := ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	// Output should contain the approval JSON
	if entry.Output == "" {
		t.Error("Expected Output to be non-empty")
	}

	// Output should match result.Output
	if result.Output == "" {
		t.Error("Expected result.Output to be non-empty")
	}

	// Verify it's a valid approval output
	if !strings.Contains(entry.Output, `"permissionDecision":"allow"`) {
		t.Errorf("Expected Output to contain allow decision, got: %s", entry.Output)
	}
}

func TestProcessWithResultCapturesOutputOnRejection(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{"session_id":"sess-1","tool_use_id":"tool-1","cwd":"/test","tool_name":"Bash","tool_input":{"command":"curl http://example.com"}}`

	result := ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	// Output should contain the ask JSON
	if entry.Output == "" {
		t.Error("Expected Output to be non-empty")
	}

	// Output should match result.Output
	if result.Output == "" {
		t.Error("Expected result.Output to be non-empty")
	}

	// Verify it's a valid ask output
	if !strings.Contains(entry.Output, `"permissionDecision":"ask"`) {
		t.Errorf("Expected Output to contain ask decision, got: %s", entry.Output)
	}
}

func TestProcessWithResultCapturesOutputOnUnparseable(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{"session_id":"sess-1","tool_use_id":"tool-1","cwd":"/test","tool_name":"Bash","tool_input":{"command":"echo 'unclosed"}}`

	result := ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	// Output should contain the ask JSON for unparseable
	if entry.Output == "" {
		t.Error("Expected Output to be non-empty")
	}

	// Output should match result.Output
	if result.Output == "" {
		t.Error("Expected result.Output to be non-empty")
	}

	// Verify it's a valid ask output
	if !strings.Contains(entry.Output, `"permissionDecision":"ask"`) {
		t.Errorf("Expected Output to contain ask decision, got: %s", entry.Output)
	}
}

func TestResultOutputFieldPopulated(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{"session_id":"sess-1","tool_use_id":"tool-1","cwd":"/test","tool_name":"Bash","tool_input":{"command":"ls"}}`

	result := ProcessWithResult(strings.NewReader(input))

	// Result.Output should have the output JSON (without trailing newline for storage)
	if result.Output == "" {
		t.Error("Expected result.Output to be non-empty")
	}

	// Should be valid JSON
	var output Output
	// Strip trailing newline if present for parsing
	outputStr := strings.TrimSuffix(result.Output, "\n")
	if err := json.Unmarshal([]byte(outputStr), &output); err != nil {
		t.Errorf("Failed to parse result.Output as JSON: %v", err)
	}

	_ = logPath // Used by setupTestAudit
}

func TestProcessWithResultAuditConfigPath(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[commands]
[[commands.simple]]
name = "ls"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{"session_id":"sess-1","tool_use_id":"tool-1","cwd":"/test","tool_name":"Bash","tool_input":{"command":"ls"}}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	if entry.ConfigPath == "" {
		t.Error("Expected ConfigPath to be non-empty")
	}
	if !strings.HasSuffix(entry.ConfigPath, "config.toml") {
		t.Errorf("ConfigPath = %q, want path ending in config.toml", entry.ConfigPath)
	}
	if entry.ConfigError != "" {
		t.Errorf("ConfigError = %q, want empty string for valid config", entry.ConfigError)
	}
}

func TestProcessWithResultAuditConfigPathEmptyWhenConfigDirFails(t *testing.T) {
	config.Reset()

	// Unset both MMI_CONFIG and HOME so GetConfigDir() fails
	origConfig := os.Getenv("MMI_CONFIG")
	origHome := os.Getenv("HOME")
	os.Unsetenv("MMI_CONFIG")
	os.Unsetenv("HOME")
	defer func() {
		os.Setenv("MMI_CONFIG", origConfig)
		os.Setenv("HOME", origHome)
		config.Reset()
	}()

	config.Init()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{"session_id":"sess-1","tool_use_id":"tool-1","cwd":"/test","tool_name":"Bash","tool_input":{"command":"ls"}}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	// When GetConfigDir fails, no config path can be determined
	if entry.ConfigPath != "" {
		t.Errorf("Expected empty ConfigPath when GetConfigDir fails, got %q", entry.ConfigPath)
	}
	// But there should be a config error
	if entry.ConfigError == "" {
		t.Error("Expected ConfigError to be non-empty when GetConfigDir fails")
	}
}

func TestProcessWithResultAuditConfigErrorOnInvalidConfig(t *testing.T) {
	config.Reset()

	// Set up a directory with invalid TOML
	tmpDir, err := os.MkdirTemp("", "mmi-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origConfig := os.Getenv("MMI_CONFIG")
	os.Setenv("MMI_CONFIG", tmpDir)
	defer func() {
		os.Setenv("MMI_CONFIG", origConfig)
		config.Reset()
	}()

	invalidConfig := `bad toml {{`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	config.Init()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{"session_id":"sess-1","tool_use_id":"tool-1","cwd":"/test","tool_name":"Bash","tool_input":{"command":"ls"}}`

	ProcessWithResult(strings.NewReader(input))

	entry := readLastAuditEntry(t, logPath)

	if entry.ConfigPath == "" {
		t.Error("Expected ConfigPath to be non-empty even with invalid config")
	}
	if entry.ConfigError == "" {
		t.Error("Expected ConfigError to be non-empty for invalid config")
	}
	if !strings.Contains(entry.ConfigError, "failed to load config") {
		t.Errorf("ConfigError = %q, want error containing 'failed to load config'", entry.ConfigError)
	}
}
