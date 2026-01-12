package cmd

import (
	"fmt"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration and show compiled patterns",
	Long: `Validate validates the mmi configuration file and displays all compiled patterns.

This is useful for:
- Checking that your config.toml syntax is correct
- Seeing what patterns will actually be used
- Debugging pattern matching issues`,
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load configuration")
	}

	fmt.Println("Configuration valid!")
	fmt.Println()

	// Show deny patterns
	fmt.Printf("Deny patterns: %d\n", len(cfg.DenyPatterns))
	for _, p := range cfg.DenyPatterns {
		fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
	}
	fmt.Println()

	// Show wrapper patterns
	fmt.Printf("Wrapper patterns: %d\n", len(cfg.WrapperPatterns))
	for _, p := range cfg.WrapperPatterns {
		fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
	}
	fmt.Println()

	// Show safe command patterns
	fmt.Printf("Safe command patterns: %d\n", len(cfg.SafeCommands))
	for _, p := range cfg.SafeCommands {
		fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
	}

	return nil
}
