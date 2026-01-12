package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/spf13/cobra"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new mmi configuration file",
	Long: `Initialize creates a new mmi configuration file with default settings.

The config file is written to ~/.config/mmi/config.toml (or the path
specified by MMI_CONFIG environment variable).

Use --force to overwrite an existing configuration file.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing config file")
}

func runInit(cmd *cobra.Command, args []string) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.toml")

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil && !initForce {
		return fmt.Errorf("config file already exists at %s (use --force to overwrite)", configPath)
	}

	// Create directory if needed
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write default config file
	if err := os.WriteFile(configPath, config.GetDefaultConfig(), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Configuration written to: %s\n", configPath)
	fmt.Println("Run 'mmi validate' to verify your configuration.")

	return nil
}
