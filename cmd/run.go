package cmd

import (
	"fmt"
	"os"

	"github.com/dgerlanc/mmi/internal/hook"
	"github.com/spf13/cobra"
)

// runHook is the default command that processes stdin for command approval
func runHook(cmd *cobra.Command, args []string) {
	// Process the command
	result := hook.ProcessWithResult(os.Stdin)

	if dryRun {
		// In dry-run mode, output to stderr instead of JSON to stdout
		if result.Approved {
			fmt.Fprintf(os.Stderr, "APPROVED: %s (reason: %s)\n", result.Command, result.Reason)
		} else if result.Command != "" {
			fmt.Fprintf(os.Stderr, "REJECTED: %s\n", result.Command)
		} else {
			fmt.Fprintf(os.Stderr, "REJECTED: (no command parsed)\n")
		}
		return
	}

	// Normal mode: output JSON decision to stdout
	fmt.Print(result.Output)
}
