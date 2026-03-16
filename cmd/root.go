// Package cmd implements the CLI commands for mmi.
package cmd

import (
	"fmt"
	"os"

	"github.com/dgerlanc/mmi/internal/audit"
	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/hook"
	"github.com/dgerlanc/mmi/internal/logger"
	"github.com/spf13/cobra"
)

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return buildRootCmd().Execute()
}

func buildRootCmd() *cobra.Command {
	var (
		verbose    bool
		dryRun     bool
		noAuditLog bool
		cfg        *config.Config
		cfgPath    string
		cfgErr     error
	)

	rootCmd := &cobra.Command{
		Use:   "mmi",
		Short: "Mother May I? - Claude Code Bash command approval hook",
		Long: `MMI (Mother May I?) is a PreToolUse hook for Claude Code that
approves or rejects Bash commands based on configurable patterns.

When called without arguments, it reads a JSON command from stdin and outputs
an the approval decision to stdout as JSON.

Usage in ~/.claude/settings.json:
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command", "command": "mmi"}]
    }]
  }`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logger.Init(logger.Options{Verbose: verbose})
			cfg, cfgPath, cfgErr = config.Load()
			audit.Init("", noAuditLog)
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			result := hook.ProcessWithResult(os.Stdin, cfg, cfgPath, cfgErr)

			if dryRun {
				if result.Approved {
					fmt.Fprintf(os.Stderr, "APPROVED: %s (reason: %s)\n", result.Command, result.Reason)
				} else if result.Command != "" {
					fmt.Fprintf(os.Stderr, "REJECTED: %s\n", result.Command)
				} else {
					fmt.Fprintf(os.Stderr, "REJECTED: (no command parsed)\n")
				}
				return
			}

			fmt.Print(result.Output)
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output (debug logging)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Test command approval without JSON output")
	rootCmd.PersistentFlags().BoolVar(&noAuditLog, "no-audit-log", false, "Disable audit logging")

	rootCmd.AddCommand(buildValidateCmd(&cfg, &cfgPath, &cfgErr))
	rootCmd.AddCommand(buildInitCmd())
	rootCmd.AddCommand(buildCompletionCmd(rootCmd))

	return rootCmd
}
