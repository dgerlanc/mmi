package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dgerlanc/mmi/internal/testutil"
	"github.com/spf13/cobra"
)

// setupTestConfig initializes a test configuration using shared test utilities
func setupTestConfig(t *testing.T) func() {
	t.Helper()
	resetGlobalState()

	cleanup := testutil.SetupTestConfig(t, testutil.MinimalTestConfig)

	return func() {
		cleanup()
		resetGlobalState()
	}
}

func TestRunHookDryRunApproved(t *testing.T) {
	cleanup := setupTestConfig(t)
	defer cleanup()

	// Set dry-run mode
	dryRun = true
	defer func() { dryRun = false }()

	// Create input JSON for a safe command
	input := `{"tool_name":"Bash","tool_input":{"command":"ls -la"}}`

	// Capture stderr (dry-run outputs to stderr)
	oldStdin := os.Stdin
	oldStderr := os.Stderr

	// Create stdin with input
	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	// Create stderr capture
	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	// Run the hook
	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	// Restore
	os.Stdin = oldStdin
	stderrW.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, stderrR)
	output := buf.String()

	// Check output contains APPROVED
	if !strings.Contains(output, "APPROVED") {
		t.Errorf("expected 'APPROVED' in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "ls -la") {
		t.Errorf("expected command 'ls -la' in output, got: %s", output)
	}
}

func TestRunHookDryRunRejected(t *testing.T) {
	cleanup := setupTestConfig(t)
	defer cleanup()

	// Set dry-run mode
	dryRun = true
	defer func() { dryRun = false }()

	// Create input JSON for an unsafe command
	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`

	// Capture stderr
	oldStdin := os.Stdin
	oldStderr := os.Stderr

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stderrW.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, stderrR)
	output := buf.String()

	// Check output contains REJECTED
	if !strings.Contains(output, "REJECTED") {
		t.Errorf("expected 'REJECTED' in dry-run output, got: %s", output)
	}
}

func TestRunHookDryRunEmptyCommand(t *testing.T) {
	cleanup := setupTestConfig(t)
	defer cleanup()

	// Set dry-run mode
	dryRun = true
	defer func() { dryRun = false }()

	// Create input JSON with empty command
	// Empty commands are approved (they're trivially safe - nothing to execute)
	input := `{"tool_name":"Bash","tool_input":{"command":""}}`

	// Capture stderr
	oldStdin := os.Stdin
	oldStderr := os.Stderr

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stderrW.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, stderrR)
	output := buf.String()

	// Empty commands are approved (considered safe as there's nothing to execute)
	if !strings.Contains(output, "APPROVED") {
		t.Errorf("expected 'APPROVED' in output for empty command, got: %s", output)
	}
}

func TestRunHookNormalModeApproved(t *testing.T) {
	cleanup := setupTestConfig(t)
	defer cleanup()

	// Ensure not in dry-run mode
	dryRun = false

	// Create input JSON for a safe command
	input := `{"tool_name":"Bash","tool_input":{"command":"ls"}}`

	// Capture stdout (normal mode outputs to stdout)
	oldStdin := os.Stdin
	oldStdout := os.Stdout

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stdoutW.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, stdoutR)
	output := buf.String()

	// Check output is JSON with approval
	if !strings.Contains(output, "hookSpecificOutput") {
		t.Errorf("expected JSON output with 'hookSpecificOutput', got: %s", output)
	}
	if !strings.Contains(output, "allow") {
		t.Errorf("expected 'allow' in JSON output, got: %s", output)
	}
}

func TestRunHookNormalModeRejectedSilent(t *testing.T) {
	cleanup := setupTestConfig(t)
	defer cleanup()

	// Ensure not in dry-run mode
	dryRun = false

	// Create input JSON for an unsafe command
	input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`

	// Capture stdout
	oldStdin := os.Stdin
	oldStdout := os.Stdout

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stdoutW.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, stdoutR)
	output := buf.String()

	// Commands matching deny list produce deny JSON output
	if output == "" {
		t.Errorf("expected deny output for rejected command, got nothing")
	}
	if !strings.Contains(output, `"permissionDecision":"deny"`) {
		t.Errorf("expected deny permission decision, got: %s", output)
	}
}

func TestRunHookInvalidJSON(t *testing.T) {
	cleanup := setupTestConfig(t)
	defer cleanup()

	// Set dry-run mode to see output
	dryRun = true
	defer func() { dryRun = false }()

	// Create invalid JSON input
	input := `{invalid json}`

	// Capture stderr
	oldStdin := os.Stdin
	oldStderr := os.Stderr

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stderrW.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, stderrR)
	output := buf.String()

	// Invalid JSON should result in rejection
	if !strings.Contains(output, "REJECTED") {
		t.Errorf("expected 'REJECTED' for invalid JSON, got: %s", output)
	}
}

func TestRunHookNonBashTool(t *testing.T) {
	cleanup := setupTestConfig(t)
	defer cleanup()

	// Set dry-run mode to see output
	dryRun = true
	defer func() { dryRun = false }()

	// Create input JSON for non-Bash tool
	input := `{"tool_name":"Write","tool_input":{"path":"/tmp/test"}}`

	// Capture stderr
	oldStdin := os.Stdin
	oldStderr := os.Stderr

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stderrW.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, stderrR)
	output := buf.String()

	// Non-Bash tool should result in rejection (no command parsed)
	if !strings.Contains(output, "REJECTED") {
		t.Errorf("expected 'REJECTED' for non-Bash tool, got: %s", output)
	}
}
