// Package cmd implements the CLI commands for mmi.
package cmd

import (
	"os"

	"github.com/dgerlanc/mmi/internal/audit"
	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/logger"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose    bool
	dryRun     bool
	profile    string
	noAuditLog bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mmi",
	Short: "Mother May I? - Claude Code Bash command approval hook",
	Long: `MMI (Mother May I?) is a PreToolUse hook for Claude Code that auto-approves
safe Bash commands based on configurable patterns.

When called without arguments, it reads a JSON command from stdin and outputs
an approval JSON to stdout if the command is safe.

Usage in ~/.claude/settings.json:
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command", "command": "mmi"}]
    }]
  }`,
	// Run the hook by default when no subcommand is given
	Run: runHook,
	// Silence usage on errors
	SilenceUsage: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Initialize before running any command
	cobra.OnInitialize(initApp)

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output (debug logging)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Test command approval without JSON output")
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "", "Config profile to use (or set MMI_PROFILE env var)")
	rootCmd.PersistentFlags().BoolVar(&noAuditLog, "no-audit-log", false, "Disable audit logging")
}

// initApp initializes the application (logger, config, audit)
func initApp() {
	// Check for profile from env var if not set via flag
	if profile == "" {
		profile = os.Getenv("MMI_PROFILE")
	}

	// Initialize logger
	logger.Init(logger.Options{Verbose: verbose})

	// Set profile before initializing config
	if profile != "" {
		config.SetProfile(profile)
	}

	// Initialize config
	config.Init()

	// Initialize audit logging (unless disabled)
	audit.Init("", noAuditLog)
}

// IsVerbose returns whether verbose mode is enabled
func IsVerbose() bool {
	return verbose
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// GetProfile returns the current profile name
func GetProfile() string {
	return profile
}
