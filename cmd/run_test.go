package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dgerlanc/mmi/internal/testutil"
)

// setupTestConfigDir creates a temp config directory with the given config content
// and sets MMI_CONFIG to point to it.
func setupTestConfigDir(t *testing.T, configContent string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunHookDryRunApproved(t *testing.T) {
	setupTestConfigDir(t, testutil.MinimalTestConfig)

	input := `{"tool_name":"Bash","tool_input":{"command":"ls -la"}}`

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--dry-run", "--no-audit-log"})
	rootCmd.SetIn(strings.NewReader(input))

	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)

	rootCmd.Execute()
	output := stderr.String()

	if !strings.Contains(output, "APPROVED") {
		t.Errorf("expected 'APPROVED' in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "ls -la") {
		t.Errorf("expected command 'ls -la' in output, got: %s", output)
	}
}

func TestRunHookDryRunRejected(t *testing.T) {
	setupTestConfigDir(t, testutil.MinimalTestConfig)

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--dry-run", "--no-audit-log"})
	rootCmd.SetIn(strings.NewReader(input))

	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)

	rootCmd.Execute()
	output := stderr.String()

	if !strings.Contains(output, "REJECTED") {
		t.Errorf("expected 'REJECTED' in dry-run output, got: %s", output)
	}
}

func TestRunHookDryRunEmptyCommand(t *testing.T) {
	setupTestConfigDir(t, testutil.MinimalTestConfig)

	input := `{"tool_name":"Bash","tool_input":{"command":""}}`

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--dry-run", "--no-audit-log"})
	rootCmd.SetIn(strings.NewReader(input))

	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)

	rootCmd.Execute()
	output := stderr.String()

	// Empty commands are approved (considered safe as there's nothing to execute)
	if !strings.Contains(output, "APPROVED") {
		t.Errorf("expected 'APPROVED' in output for empty command, got: %s", output)
	}
}

func TestRunHookNormalModeApproved(t *testing.T) {
	setupTestConfigDir(t, testutil.MinimalTestConfig)

	input := `{"tool_name":"Bash","tool_input":{"command":"ls"}}`

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--no-audit-log"})
	rootCmd.SetIn(strings.NewReader(input))

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	rootCmd.Execute()
	output := stdout.String()

	if !strings.Contains(output, "hookSpecificOutput") {
		t.Errorf("expected JSON output with 'hookSpecificOutput', got: %s", output)
	}
	if !strings.Contains(output, "allow") {
		t.Errorf("expected 'allow' in JSON output, got: %s", output)
	}
}

func TestRunHookNormalModeRejectedSilent(t *testing.T) {
	setupTestConfigDir(t, testutil.MinimalTestConfig)

	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--no-audit-log"})
	rootCmd.SetIn(strings.NewReader(input))

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	rootCmd.Execute()
	output := stdout.String()

	// Commands matching deny list produce deny JSON output
	if output == "" {
		t.Errorf("expected deny output for rejected command, got nothing")
	}
	if !strings.Contains(output, `"permissionDecision":"deny"`) {
		t.Errorf("expected deny permission decision, got: %s", output)
	}
}

func TestRunHookInvalidJSON(t *testing.T) {
	setupTestConfigDir(t, testutil.MinimalTestConfig)

	input := `{invalid json}`

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--dry-run", "--no-audit-log"})
	rootCmd.SetIn(strings.NewReader(input))

	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)

	rootCmd.Execute()
	output := stderr.String()

	// Invalid JSON should result in rejection
	if !strings.Contains(output, "REJECTED") {
		t.Errorf("expected 'REJECTED' for invalid JSON, got: %s", output)
	}
}

func TestRunHookNonBashTool(t *testing.T) {
	setupTestConfigDir(t, testutil.MinimalTestConfig)

	input := `{"tool_name":"Write","tool_input":{"path":"/tmp/test"}}`

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--dry-run", "--no-audit-log"})
	rootCmd.SetIn(strings.NewReader(input))

	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)

	rootCmd.Execute()
	output := stderr.String()

	// Non-Bash tool should result in rejection (no command parsed)
	if !strings.Contains(output, "REJECTED") {
		t.Errorf("expected 'REJECTED' for non-Bash tool, got: %s", output)
	}
}
