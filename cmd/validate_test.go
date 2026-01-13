package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/spf13/cobra"
)

func TestRunValidateWithValidConfig(t *testing.T) {
	resetGlobalState()

	// Create temp directory with valid config
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Write a valid config file
	validConfig := `
[[deny.simple]]
name = "dangerous"
commands = ["rm"]

[[wrappers.simple]]
name = "env"
commands = ["env"]

[[commands.simple]]
name = "safe"
commands = ["ls", "cat"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(validConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize config
	config.Reset()
	config.Init()

	// Capture stdout by redirecting it
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create a command for testing
	cmd := &cobra.Command{}

	// Run validate
	err := runValidate(cmd, []string{})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	// Check output contains expected sections
	expectedStrings := []string{
		"Configuration valid!",
		"Deny patterns:",
		"Wrapper patterns:",
		"Safe command patterns:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("output should contain %q, got:\n%s", expected, output)
		}
	}
}

func TestRunValidateShowsPatternCounts(t *testing.T) {
	resetGlobalState()

	// Create temp directory with config
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Write config with known number of patterns
	testConfig := `
[[deny.simple]]
name = "deny1"
commands = ["rm"]

[[deny.simple]]
name = "deny2"
commands = ["sudo"]

[[wrappers.simple]]
name = "wrapper1"
commands = ["env"]

[[commands.simple]]
name = "cmd1"
commands = ["ls"]

[[commands.simple]]
name = "cmd2"
commands = ["cat", "head"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize config
	config.Reset()
	config.Init()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := runValidate(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	// Check pattern counts are displayed
	if !strings.Contains(output, "Deny patterns: 2") {
		t.Errorf("expected 'Deny patterns: 2' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Wrapper patterns: 1") {
		t.Errorf("expected 'Wrapper patterns: 1' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Safe command patterns: 3") {
		t.Errorf("expected 'Safe command patterns: 3' in output, got:\n%s", output)
	}
}

func TestRunValidateShowsPatternNames(t *testing.T) {
	resetGlobalState()

	// Create temp directory with config
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Write config with named patterns
	// Note: For simple entries (commands/wrappers), the pattern name is derived
	// from the command itself, not the "name" field. The "name" field is just
	// for human-readable identification in configuration.
	testConfig := `
[[deny.simple]]
name = "my-deny-pattern"
commands = ["rm"]

[[wrappers.simple]]
name = "my-wrapper-pattern"
commands = ["env"]

[[commands.simple]]
name = "my-command-pattern"
commands = ["ls"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize config
	config.Reset()
	config.Init()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := runValidate(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	// Check pattern names are displayed - for simple entries, name is derived from command
	if !strings.Contains(output, "my-deny-pattern") {
		t.Errorf("expected 'my-deny-pattern' in output, got:\n%s", output)
	}
	// Wrapper simple entries use the command name as pattern name
	if !strings.Contains(output, "env") {
		t.Errorf("expected 'env' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "my-command-pattern") {
		t.Errorf("expected 'my-command-pattern' in output, got:\n%s", output)
	}
}

func TestValidateCmdUsage(t *testing.T) {
	if validateCmd.Use != "validate" {
		t.Errorf("validateCmd.Use = %q, want 'validate'", validateCmd.Use)
	}

	if validateCmd.Short == "" {
		t.Error("validateCmd.Short should not be empty")
	}

	if validateCmd.Long == "" {
		t.Error("validateCmd.Long should not be empty")
	}
}

func TestRunValidateWithEmptyConfig(t *testing.T) {
	resetGlobalState()

	// Create temp directory with minimal valid config (empty sections)
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Write a minimal config with required field
	emptyConfig := `
[[commands.simple]]
name = "minimal"
commands = ["true"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(emptyConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize config
	config.Reset()
	config.Init()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := runValidate(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	// Should show zero counts for deny and wrapper patterns
	if !strings.Contains(output, "Deny patterns: 0") {
		t.Errorf("expected 'Deny patterns: 0' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Wrapper patterns: 0") {
		t.Errorf("expected 'Wrapper patterns: 0' in output, got:\n%s", output)
	}
}
