package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunValidateWithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

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

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := stdout.String()

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
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

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

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := stdout.String()

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
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

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

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := stdout.String()

	if !strings.Contains(output, "my-deny-pattern") {
		t.Errorf("expected 'my-deny-pattern' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "env") {
		t.Errorf("expected 'env' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "my-command-pattern") {
		t.Errorf("expected 'my-command-pattern' in output, got:\n%s", output)
	}
}

func TestRunValidateWithInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	invalidConfig := `
[[commands.simple]]
name = "test"
commands = ["foo""]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(invalidConfig), 0644); err != nil {
		t.Fatal(err)
	}

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	rootCmd.SilenceErrors = true

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("Execute() should return error for invalid config")
	}

	if !strings.Contains(err.Error(), "configuration error") {
		t.Errorf("error should contain 'configuration error', got: %v", err)
	}
}

func TestRunValidateWithMissingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	// No config file written - missing config now succeeds with zero patterns
	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute() should succeed with missing config, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Configuration valid!") {
		t.Errorf("expected 'Configuration valid!' in output, got:\n%s", output)
	}
}

func TestValidateCmdUsage(t *testing.T) {
	rootCmd := buildRootCmd()
	var validateCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "validate" {
			validateCmd = cmd
			break
		}
	}
	if validateCmd == nil {
		t.Fatal("validate subcommand not found")
	}

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

func TestRunValidateShowsSubshellAllowAll(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	testConfig := `
[subshell]
allow_all = true

[[commands.simple]]
name = "test"
commands = ["ls"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Subshell allow all: true") {
		t.Errorf("expected 'Subshell allow all: true' in output, got:\n%s", output)
	}
}

func TestRunValidateShowsSubshellAllowAllFalse(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	testConfig := `
[[commands.simple]]
name = "test"
commands = ["ls"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Subshell allow all: false") {
		t.Errorf("expected 'Subshell allow all: false' in output, got:\n%s", output)
	}
}

func TestRunValidateWithEmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	emptyConfig := `
[[commands.simple]]
name = "minimal"
commands = ["true"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(emptyConfig), 0644); err != nil {
		t.Fatal(err)
	}

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"validate", "--no-audit-log"})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Deny patterns: 0") {
		t.Errorf("expected 'Deny patterns: 0' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Wrapper patterns: 0") {
		t.Errorf("expected 'Wrapper patterns: 0' in output, got:\n%s", output)
	}
}
