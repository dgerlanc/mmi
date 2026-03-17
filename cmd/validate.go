package cmd

import (
	"fmt"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/spf13/cobra"
)

func buildValidateCmd(cfg **config.Config, cfgPath *string, cfgErr *error) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and show compiled patterns",
		Long: `Validate validates the mmi configuration file and displays all compiled patterns.

This is useful for:
- Checking that your config.toml syntax is correct
- Seeing what patterns will actually be used
- Debugging pattern matching issues`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if *cfgErr != nil {
				return fmt.Errorf("configuration error: %w", *cfgErr)
			}

			w := cmd.OutOrStdout()

			fmt.Fprintln(w, "Configuration valid!")
			fmt.Fprintln(w)

			fmt.Fprintf(w, "Subshell allow all: %v\n", (*cfg).SubshellAllowAll)
			fmt.Fprintln(w)

			fmt.Fprintf(w, "Deny patterns: %d\n", len((*cfg).DenyPatterns))
			for _, p := range (*cfg).DenyPatterns {
				fmt.Fprintf(w, "  - %s: %s\n", p.Name, p.Regex.String())
			}
			fmt.Fprintln(w)

			fmt.Fprintf(w, "Wrapper patterns: %d\n", len((*cfg).WrapperPatterns))
			for _, p := range (*cfg).WrapperPatterns {
				fmt.Fprintf(w, "  - %s: %s\n", p.Name, p.Regex.String())
			}
			fmt.Fprintln(w)

			fmt.Fprintf(w, "Safe command patterns: %d\n", len((*cfg).SafeCommands))
			for _, p := range (*cfg).SafeCommands {
				fmt.Fprintf(w, "  - %s: %s\n", p.Name, p.Regex.String())
			}

			return nil
		},
	}
}
