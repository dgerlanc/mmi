package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/constants"
	"github.com/dgerlanc/mmi/internal/hook"
	"github.com/spf13/cobra"
)

func buildInitCmd() *cobra.Command {
	var (
		force          bool
		configOnly     bool
		claudeSettings string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new mmi configuration file",
		Long: `Initialize creates a new mmi configuration file with default settings.

The config file is written to ~/.config/mmi/config.toml (or the path
specified by MMI_CONFIG environment variable).

By default, this command also configures Claude Code's settings.json to add
the mmi PreToolUse hook for Bash commands. This enables mmi to intercept
and validate commands before execution.

Use --force to overwrite an existing configuration file.
Use --config-only to skip configuring Claude Code settings.
Use --claude-settings to specify a custom path to Claude's settings.json.`,
		RunE: func(c *cobra.Command, args []string) error {
			return runInit(c, args, force, configOnly, claudeSettings)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing config file")
	cmd.Flags().BoolVar(&configOnly, "config-only", false, "Only write config.toml, skip Claude settings")
	cmd.Flags().StringVar(&claudeSettings, "claude-settings", "", "Path to Claude settings.json (default: ~/.claude/settings.json)")

	return cmd
}

func runInit(cmd *cobra.Command, args []string, force, configOnly bool, claudeSettings string) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	configPath := filepath.Join(configDir, constants.ConfigFileName)

	// Check if config already exists
	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
	}

	// Handle config file creation/update
	if configExists && !force {
		fmt.Printf("Config file already exists at %s (use --force to overwrite)\n", configPath)
	} else {
		// Create directory if needed
		if err := os.MkdirAll(configDir, constants.DirMode); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write default config file
		if err := os.WriteFile(configPath, config.GetDefaultConfig(), constants.FileMode); err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}

		fmt.Printf("Configuration written to: %s\n", configPath)
		fmt.Println("Run 'mmi validate' to verify your configuration.")
	}

	// Configure Claude settings unless --config-only was passed
	if !configOnly {
		if err := configureClaudeSettings(claudeSettings); err != nil {
			return err
		}
	}

	return nil
}

// getClaudeSettingsPath returns the path to Claude's settings.json file.
// It checks the --claude-settings flag first, then falls back to
// ~/.claude/settings.json.
func getClaudeSettingsPath(claudeSettings string) (string, error) {
	if claudeSettings != "" {
		return claudeSettings, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, constants.ClaudeConfigDir, constants.ClaudeSettingsFile), nil
}

// isMMIHookPresent checks if the mmi hook is already configured in the settings.
// It looks for a Bash matcher in hooks.PreToolUse that has an mmi command hook.
func isMMIHookPresent(settings map[string]any) bool {
	if settings == nil {
		return false
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}

	preToolUse, ok := hooks[hook.EventPreToolUse].([]any)
	if !ok {
		return false
	}

	for _, matcher := range preToolUse {
		m, ok := matcher.(map[string]any)
		if !ok {
			continue
		}

		if m["matcher"] != hook.ToolNameBash {
			continue
		}

		hooksList, ok := m["hooks"].([]any)
		if !ok {
			continue
		}

		for _, hk := range hooksList {
			h, ok := hk.(map[string]any)
			if !ok {
				continue
			}

			if h["type"] == "command" && h["command"] == constants.AppName {
				return true
			}
		}
	}

	return false
}

// addMMIHook adds the mmi hook to the settings.
// It preserves all existing settings and hooks.
func addMMIHook(settings map[string]any) map[string]any {
	if settings == nil {
		settings = make(map[string]any)
	}

	// Ensure hooks map exists
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}

	// Ensure PreToolUse array exists
	preToolUse, ok := hooks[hook.EventPreToolUse].([]any)
	if !ok {
		preToolUse = []any{}
	}

	// Create the mmi hook entry
	mmiHook := map[string]any{
		"matcher": hook.ToolNameBash,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": constants.AppName,
			},
		},
	}

	// Append the new matcher
	preToolUse = append(preToolUse, mmiHook)
	hooks[hook.EventPreToolUse] = preToolUse

	return settings
}

// configureClaudeSettings adds the mmi hook to Claude's settings.json.
// It preserves existing settings and only adds the hook if not already present.
func configureClaudeSettings(claudeSettings string) error {
	settingsPath, err := getClaudeSettingsPath(claudeSettings)
	if err != nil {
		return err
	}

	// Read existing settings or start with empty map
	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse Claude settings.json: %w", err)
		}
	} else if os.IsNotExist(err) {
		settings = make(map[string]any)
	} else {
		return fmt.Errorf("failed to read Claude settings.json: %w", err)
	}

	// Check if hook is already present
	if isMMIHookPresent(settings) {
		fmt.Printf("Claude Code hook already configured in: %s\n", settingsPath)
		return nil
	}

	// Add the hook
	settings = addMMIHook(settings)

	// Create directory if needed
	settingsDir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(settingsDir, constants.DirMode); err != nil {
		return fmt.Errorf("failed to create Claude settings directory: %w", err)
	}

	// Write back with 2-space indentation
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Claude settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, constants.FileMode); err != nil {
		return fmt.Errorf("failed to write Claude settings.json: %w", err)
	}

	fmt.Printf("Claude Code hook configured in: %s\n", settingsPath)
	return nil
}
