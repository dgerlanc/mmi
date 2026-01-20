package cmd

import (
	"bytes"
	"encoding/json"
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

func TestRunInitWithExistingConfigPrintsNotice(t *testing.T) {
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

	// Set up Claude settings path
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	initClaudeSettings = settingsPath

	// Create a command for testing
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Reset flags
	initForce = false
	initConfigOnly = false

	// Run init - should succeed (prints notice but no error)
	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("expected no error when config exists, got: %v", err)
	}

	// Verify original content was not modified
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if !bytes.Equal(content, existingContent) {
		t.Error("existing config file was modified")
	}

	// Verify Claude settings were configured
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("settings.json was not created")
	}

	// Verify mmi hook is in settings
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	if !isMMIHookPresent(settings) {
		t.Error("mmi hook not found in settings.json")
	}
}

func TestRunInitWithExistingConfigConfiguresClaudeSettings(t *testing.T) {
	resetGlobalState()

	// Create temp directory with existing config
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Create existing config file with custom content
	configPath := filepath.Join(tmpDir, "config.toml")
	existingContent := []byte("# custom config that should be preserved\n[patterns]\nallowed = [\"echo *\"]\n")
	if err := os.WriteFile(configPath, existingContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Set up Claude settings path with empty settings file
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	initClaudeSettings = settingsPath

	// Create empty settings.json
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Reset flags - no force, no config-only
	initForce = false
	initConfigOnly = false

	// Run init without --force
	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify config.toml was NOT modified
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if !bytes.Equal(content, existingContent) {
		t.Errorf("config.toml was modified:\ngot: %s\nwant: %s", content, existingContent)
	}

	// Verify settings.json now has mmi hook
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	if !isMMIHookPresent(settings) {
		t.Error("mmi hook not found in settings.json after init without --force")
	}
}

func TestRunInitWithExistingConfigAndConfigOnlySkipsClaudeSettings(t *testing.T) {
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

	// Set up Claude settings path
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	initClaudeSettings = settingsPath

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Set config-only flag
	initForce = false
	initConfigOnly = true
	defer func() { initConfigOnly = false }()

	// Run init with --config-only
	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify config.toml was NOT modified
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if !bytes.Equal(content, existingContent) {
		t.Error("config.toml was modified with --config-only")
	}

	// Verify settings.json was NOT created (--config-only skips Claude settings)
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("settings.json should not be created with --config-only")
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

// Tests for Claude settings functionality

func TestInitCmdHasConfigOnlyFlag(t *testing.T) {
	flag := initCmd.Flags().Lookup("config-only")
	if flag == nil {
		t.Fatal("init command should have --config-only flag")
	}

	if flag.DefValue != "false" {
		t.Errorf("--config-only flag default = %q, want 'false'", flag.DefValue)
	}
}

func TestInitCmdHasClaudeSettingsFlag(t *testing.T) {
	flag := initCmd.Flags().Lookup("claude-settings")
	if flag == nil {
		t.Fatal("init command should have --claude-settings flag")
	}

	if flag.DefValue != "" {
		t.Errorf("--claude-settings flag default = %q, want empty string", flag.DefValue)
	}
}

func TestRunInitWithConfigOnlySkipsSettings(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")
	claudeDir := filepath.Join(tmpDir, ".claude")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")
	initClaudeSettings = filepath.Join(claudeDir, "settings.json")

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	initForce = false
	initConfigOnly = true
	defer func() { initConfigOnly = false }()

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify config.toml was created
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Verify settings.json was NOT created
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("settings.json should not be created with --config-only")
	}
}

func TestRunInitConfiguresClaudeSettings(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")
	initClaudeSettings = settingsPath

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	initForce = false
	initConfigOnly = false

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify config.toml was created
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Verify settings.json was created
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("settings.json was not created")
	}

	// Verify mmi hook is in settings
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	if !isMMIHookPresent(settings) {
		t.Error("mmi hook not found in settings.json")
	}
}

func TestRunInitPreservesExistingSettings(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")
	initClaudeSettings = settingsPath

	// Create existing settings with other keys
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	existingSettings := map[string]any{
		"someOtherSetting": "value",
		"nested": map[string]any{
			"key": "value",
		},
	}
	data, _ := json.MarshalIndent(existingSettings, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	initForce = false
	initConfigOnly = false

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Read back settings
	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	// Verify existing settings are preserved
	if settings["someOtherSetting"] != "value" {
		t.Error("existing setting 'someOtherSetting' was not preserved")
	}

	nested, ok := settings["nested"].(map[string]any)
	if !ok || nested["key"] != "value" {
		t.Error("existing nested setting was not preserved")
	}

	// Verify mmi hook was added
	if !isMMIHookPresent(settings) {
		t.Error("mmi hook was not added to existing settings")
	}
}

func TestRunInitPreservesOtherHooks(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")
	initClaudeSettings = settingsPath

	// Create existing settings with other PreToolUse hooks
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	existingSettings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Edit",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "other-tool",
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existingSettings, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	initForce = false
	initConfigOnly = false

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Read back settings
	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	// Verify mmi hook was added
	if !isMMIHookPresent(settings) {
		t.Error("mmi hook was not added")
	}

	// Verify other hooks are preserved
	hooks := settings["hooks"].(map[string]any)
	preToolUse := hooks["PreToolUse"].([]any)

	// Should have 2 matchers now
	if len(preToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse matchers, got %d", len(preToolUse))
	}

	// Find the Edit matcher
	foundEdit := false
	for _, matcher := range preToolUse {
		m := matcher.(map[string]any)
		if m["matcher"] == "Edit" {
			foundEdit = true
			break
		}
	}
	if !foundEdit {
		t.Error("existing Edit matcher was not preserved")
	}
}

func TestRunInitSkipsWhenHookPresent(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")
	initClaudeSettings = settingsPath

	// Create existing settings with mmi hook already present
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	existingSettings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "mmi",
						},
					},
				},
			},
		},
	}
	originalData, _ := json.MarshalIndent(existingSettings, "", "  ")
	if err := os.WriteFile(settingsPath, originalData, 0644); err != nil {
		t.Fatal(err)
	}

	// Get original mod time
	originalInfo, _ := os.Stat(settingsPath)
	originalModTime := originalInfo.ModTime()

	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	initForce = false
	initConfigOnly = false

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify file was not modified (by checking content is identical)
	newData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	// Check content is unchanged
	if !bytes.Equal(originalData, newData) {
		t.Error("settings.json was modified when hook was already present")
	}

	// Also check mod time wasn't updated (file wasn't rewritten)
	newInfo, _ := os.Stat(settingsPath)
	if !newInfo.ModTime().Equal(originalModTime) {
		t.Error("settings.json mod time changed when hook was already present")
	}
}

func TestRunInitCreatesClaudeDir(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")
	claudeDir := filepath.Join(tmpDir, "nested", "path", ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")
	initClaudeSettings = settingsPath

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	initForce = false
	initConfigOnly = false

	err := runInit(cmd, []string{})
	if err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	// Verify claude directory was created
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		t.Error("claude directory was not created")
	}

	// Verify settings.json was created
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("settings.json was not created")
	}
}

func TestRunInitHandlesInvalidJSON(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "mmi")
	claudeDir := filepath.Join(tmpDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	os.Setenv("MMI_CONFIG", configDir)
	defer os.Unsetenv("MMI_CONFIG")
	initClaudeSettings = settingsPath

	// Create existing settings with invalid JSON
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte("{ invalid json }"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	initForce = false
	initConfigOnly = false

	err := runInit(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "JSON") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention parsing issue, got: %v", err)
	}
}

// Unit tests for helper functions

func TestIsMMIHookPresent(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		want     bool
	}{
		{
			name:     "empty settings",
			settings: map[string]any{},
			want:     false,
		},
		{
			name:     "nil settings",
			settings: nil,
			want:     false,
		},
		{
			name: "no hooks key",
			settings: map[string]any{
				"otherKey": "value",
			},
			want: false,
		},
		{
			name: "hooks but no PreToolUse",
			settings: map[string]any{
				"hooks": map[string]any{
					"PostToolUse": []any{},
				},
			},
			want: false,
		},
		{
			name: "PreToolUse but no Bash matcher",
			settings: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Edit",
							"hooks":   []any{},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "Bash matcher but no mmi hook",
			settings: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Bash",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": "other-tool",
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "mmi hook present",
			settings: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Bash",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": "mmi",
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "mmi hook present among other hooks",
			settings: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Bash",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": "other-tool",
								},
								map[string]any{
									"type":    "command",
									"command": "mmi",
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "mmi hook in different Bash matcher",
			settings: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Edit",
							"hooks":   []any{},
						},
						map[string]any{
							"matcher": "Bash",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": "mmi",
								},
							},
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMMIHookPresent(tt.settings)
			if got != tt.want {
				t.Errorf("isMMIHookPresent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddMMIHook(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
	}{
		{
			name:     "empty settings",
			settings: map[string]any{},
		},
		{
			name:     "nil settings",
			settings: nil,
		},
		{
			name: "existing other settings",
			settings: map[string]any{
				"otherKey": "value",
			},
		},
		{
			name: "existing hooks but no PreToolUse",
			settings: map[string]any{
				"hooks": map[string]any{
					"PostToolUse": []any{
						map[string]any{
							"matcher": "Bash",
							"hooks":   []any{},
						},
					},
				},
			},
		},
		{
			name: "existing PreToolUse with other matchers",
			settings: map[string]any{
				"hooks": map[string]any{
					"PreToolUse": []any{
						map[string]any{
							"matcher": "Edit",
							"hooks": []any{
								map[string]any{
									"type":    "command",
									"command": "other-tool",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addMMIHook(tt.settings)

			// Verify the hook was added
			if !isMMIHookPresent(result) {
				t.Error("mmi hook should be present after addMMIHook")
			}

			// Verify existing settings are preserved
			if tt.settings != nil {
				if v, ok := tt.settings["otherKey"]; ok {
					if result["otherKey"] != v {
						t.Error("existing settings should be preserved")
					}
				}
			}

			// Verify existing hooks are preserved
			if tt.settings != nil {
				if hooks, ok := tt.settings["hooks"].(map[string]any); ok {
					if postToolUse, ok := hooks["PostToolUse"]; ok {
						resultHooks := result["hooks"].(map[string]any)
						if resultHooks["PostToolUse"] == nil {
							t.Error("existing PostToolUse hooks should be preserved")
						}
						_ = postToolUse // use the variable
					}
				}
			}
		})
	}
}

func TestAddMMIHookPreservesExistingMatchers(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Edit",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "other-tool",
						},
					},
				},
			},
		},
	}

	result := addMMIHook(settings)

	hooks := result["hooks"].(map[string]any)
	preToolUse := hooks["PreToolUse"].([]any)

	if len(preToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse matchers, got %d", len(preToolUse))
	}

	// Verify Edit matcher is still there
	foundEdit := false
	for _, matcher := range preToolUse {
		m := matcher.(map[string]any)
		if m["matcher"] == "Edit" {
			foundEdit = true
			break
		}
	}
	if !foundEdit {
		t.Error("existing Edit matcher should be preserved")
	}
}
