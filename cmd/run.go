package cmd

import (
	"fmt"
	"os"

	"github.com/dgerlanc/mmi/internal/hook"
	"github.com/spf13/cobra"
)

// runHook is the default command that processes stdin for command approval
func runHook(cmd *cobra.Command, args []string) {
	approved, reason := hook.Process(os.Stdin)

	if dryRun {
		// In dry-run mode, output to stderr instead of JSON to stdout
		input := getCommandFromStdin()
		if approved {
			fmt.Fprintf(os.Stderr, "APPROVED: %s (reason: %s)\n", input, reason)
		} else {
			fmt.Fprintf(os.Stderr, "REJECTED: %s\n", input)
		}
		return
	}

	// Normal mode: output JSON approval to stdout
	if approved {
		fmt.Print(hook.FormatApproval(reason))
	}
	// Silent rejection (no output) for non-approved commands
}

// getCommandFromStdin attempts to extract the command from stdin for dry-run output
// This is a best-effort extraction since stdin has already been consumed by Process
func getCommandFromStdin() string {
	// Since stdin was already consumed by Process, we can't re-read it
	// In a future improvement, we could modify Process to return the command
	return "(command from stdin)"
}
