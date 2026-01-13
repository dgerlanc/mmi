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

func TestRunInitCreatesConfigFile(t *testing.T) {
	resetGlobalState()

	// Create temp directory for config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")

	// Set environment to use temp directory
	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Create a command for testing
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Reset force flag
	initForce = false

	// Run init
	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify config file was created
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Verify content matches default config
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	expectedContent := config.GetDefaultConfig()
	if !bytes.Equal(content, expectedContent) {
		t.Error("config file content does not match default config")
	}
}

func TestRunInitFailsWhenConfigExists(t *testing.T) {
	resetGlobalState()

	// Create temp directory with existing config
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Create existing config file
	configPath := filepath.Join(tmpDir, "config.toml")
	existingContent := []byte("# existing config")
	if err := os.WriteFile(configPath, existingContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a command for testing
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Reset force flag
	initForce = false

	// Run init - should fail
	err := runInit(cmd, []string{})
	if err == nil {
		t.Fatal("expected error when config exists, got nil")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}

	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention '--force', got: %v", err)
	}

	// Verify original content was not modified
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if !bytes.Equal(content, existingContent) {
		t.Error("existing config file was modified")
	}
}

func TestRunInitWithForceOverwrites(t *testing.T) {
	resetGlobalState()

	// Create temp directory with existing config
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Create existing config file with different content
	configPath := filepath.Join(tmpDir, "config.toml")
	existingContent := []byte("# old config that should be overwritten")
	if err := os.WriteFile(configPath, existingContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a command for testing
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Set force flag
	initForce = true
	defer func() { initForce = false }()

	// Run init - should succeed with force
	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() with --force error = %v", err)
	}

	// Verify content was replaced with default config
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	expectedContent := config.GetDefaultConfig()
	if !bytes.Equal(content, expectedContent) {
		t.Error("config file was not overwritten with default config")
	}
}

func TestRunInitCreatesDirectory(t *testing.T) {
	resetGlobalState()

	// Create temp directory
	tmpDir := t.TempDir()
	// Use a nested path that doesn't exist
	configDir := filepath.Join(tmpDir, "nested", "path", "mmi")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Create a command for testing
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Reset force flag
	initForce = false

	// Run init
	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("config directory was not created")
	}

	// Verify config file exists
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestInitCmdHasForceFlag(t *testing.T) {
	flag := initCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("init command should have --force flag")
	}

	if flag.Shorthand != "f" {
		t.Errorf("--force flag shorthand = %q, want 'f'", flag.Shorthand)
	}

	if flag.DefValue != "false" {
		t.Errorf("--force flag default = %q, want 'false'", flag.DefValue)
	}
}

func TestInitCmdUsage(t *testing.T) {
	if initCmd.Use != "init" {
		t.Errorf("initCmd.Use = %q, want 'init'", initCmd.Use)
	}

	if initCmd.Short == "" {
		t.Error("initCmd.Short should not be empty")
	}

	if initCmd.Long == "" {
		t.Error("initCmd.Long should not be empty")
	}
}
