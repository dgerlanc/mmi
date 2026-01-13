package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/spf13/cobra"
)

// resetGlobalState resets all global flags to their default values
func resetGlobalState() {
	verbose = false
	dryRun = false
	profile = ""
	noAuditLog = false
	config.Reset()
}

func TestIsVerbose(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected bool
	}{
		{"verbose false", false, false},
		{"verbose true", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobalState()
			verbose = tt.value
			if got := IsVerbose(); got != tt.expected {
				t.Errorf("IsVerbose() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsDryRun(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected bool
	}{
		{"dry-run false", false, false},
		{"dry-run true", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobalState()
			dryRun = tt.value
			if got := IsDryRun(); got != tt.expected {
				t.Errorf("IsDryRun() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetProfile(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"empty profile", "", ""},
		{"named profile", "strict", "strict"},
		{"profile with dash", "my-profile", "my-profile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobalState()
			profile = tt.value
			if got := GetProfile(); got != tt.expected {
				t.Errorf("GetProfile() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestInitAppWithEnvProfile(t *testing.T) {
	resetGlobalState()

	// Create temp config directory
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Write minimal config
	configContent := `
[[commands.simple]]
name = "test"
commands = ["echo"]
`
	if err := os.WriteFile(tmpDir+"/config.toml", []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Set profile via environment variable
	os.Setenv("MMI_PROFILE", "test-profile")
	defer os.Unsetenv("MMI_PROFILE")

	// Run initApp
	initApp()

	// Verify profile was picked up from env var
	if config.GetProfile() != "test-profile" {
		t.Errorf("expected profile 'test-profile' from env var, got %q", config.GetProfile())
	}
}

func TestInitAppProfileFlagOverridesEnv(t *testing.T) {
	resetGlobalState()

	// Create temp config directory
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Write minimal config
	configContent := `
[[commands.simple]]
name = "test"
commands = ["echo"]
`
	if err := os.WriteFile(tmpDir+"/config.toml", []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Set profile via environment variable
	os.Setenv("MMI_PROFILE", "env-profile")
	defer os.Unsetenv("MMI_PROFILE")

	// Set profile via flag (simulating --profile flag)
	profile = "flag-profile"

	// Run initApp
	initApp()

	// Flag profile should be used (since it's already set, env var is not checked)
	if config.GetProfile() != "flag-profile" {
		t.Errorf("expected profile 'flag-profile' from flag, got %q", config.GetProfile())
	}
}

func TestRootCmdFlags(t *testing.T) {
	resetGlobalState()

	// Create a fresh root command for testing
	cmd := &cobra.Command{Use: "test"}
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Test command approval")
	cmd.PersistentFlags().StringVar(&profile, "profile", "", "Config profile to use")
	cmd.PersistentFlags().BoolVar(&noAuditLog, "no-audit-log", false, "Disable audit logging")

	tests := []struct {
		name          string
		args          []string
		expectVerbose bool
		expectDryRun  bool
		expectProfile string
		expectNoAudit bool
	}{
		{
			name:          "no flags",
			args:          []string{},
			expectVerbose: false,
			expectDryRun:  false,
			expectProfile: "",
			expectNoAudit: false,
		},
		{
			name:          "verbose short flag",
			args:          []string{"-v"},
			expectVerbose: true,
			expectDryRun:  false,
			expectProfile: "",
			expectNoAudit: false,
		},
		{
			name:          "verbose long flag",
			args:          []string{"--verbose"},
			expectVerbose: true,
			expectDryRun:  false,
			expectProfile: "",
			expectNoAudit: false,
		},
		{
			name:          "dry-run flag",
			args:          []string{"--dry-run"},
			expectVerbose: false,
			expectDryRun:  true,
			expectProfile: "",
			expectNoAudit: false,
		},
		{
			name:          "profile flag",
			args:          []string{"--profile", "strict"},
			expectVerbose: false,
			expectDryRun:  false,
			expectProfile: "strict",
			expectNoAudit: false,
		},
		{
			name:          "no-audit-log flag",
			args:          []string{"--no-audit-log"},
			expectVerbose: false,
			expectDryRun:  false,
			expectProfile: "",
			expectNoAudit: true,
		},
		{
			name:          "multiple flags",
			args:          []string{"-v", "--dry-run", "--profile", "test"},
			expectVerbose: true,
			expectDryRun:  true,
			expectProfile: "test",
			expectNoAudit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			verbose = false
			dryRun = false
			profile = ""
			noAuditLog = false

			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.Run = func(cmd *cobra.Command, args []string) {} // noop

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if verbose != tt.expectVerbose {
				t.Errorf("verbose = %v, want %v", verbose, tt.expectVerbose)
			}
			if dryRun != tt.expectDryRun {
				t.Errorf("dryRun = %v, want %v", dryRun, tt.expectDryRun)
			}
			if profile != tt.expectProfile {
				t.Errorf("profile = %q, want %q", profile, tt.expectProfile)
			}
			if noAuditLog != tt.expectNoAudit {
				t.Errorf("noAuditLog = %v, want %v", noAuditLog, tt.expectNoAudit)
			}
		})
	}
}

func TestRootCmdHasExpectedSubcommands(t *testing.T) {
	expectedCommands := []string{"init", "validate", "completion"}

	for _, cmdName := range expectedCommands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", cmdName)
		}
	}
}

func TestRootCmdUsageContainsDescription(t *testing.T) {
	if rootCmd.Short == "" {
		t.Error("rootCmd.Short should not be empty")
	}
	if rootCmd.Long == "" {
		t.Error("rootCmd.Long should not be empty")
	}
	if rootCmd.Use != "mmi" {
		t.Errorf("rootCmd.Use = %q, want 'mmi'", rootCmd.Use)
	}
}
