package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo", "ls"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.SafeCommands) != 2 {
		t.Errorf("expected 2 safe commands, got %d", len(cfg.SafeCommands))
	}
}

func TestLoadConfigWithIncludes(t *testing.T) {
	// Create temp directory
	dir := t.TempDir()

	// Write main config
	mainConfig := []byte(`
include = ["tools.toml"]

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Write included config
	toolsConfig := []byte(`
[[commands.simple]]
name = "tools"
commands = ["ls", "cat"]
`)
	if err := os.WriteFile(filepath.Join(dir, "tools.toml"), toolsConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Load with includes
	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}

	// Should have 3 commands: echo from main + ls, cat from tools
	if len(cfg.SafeCommands) != 3 {
		t.Errorf("expected 3 safe commands, got %d", len(cfg.SafeCommands))
	}
}

func TestLoadConfigCircularInclude(t *testing.T) {
	// Create temp directory
	dir := t.TempDir()

	// Write config that includes itself indirectly
	configA := []byte(`include = ["b.toml"]`)
	configB := []byte(`include = ["a.toml"]`)

	if err := os.WriteFile(filepath.Join(dir, "a.toml"), configA, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.toml"), configB, 0644); err != nil {
		t.Fatal(err)
	}

	// Load should fail with circular include error
	_, err := LoadConfigWithDir(configA, dir)
	if err == nil {
		t.Error("expected circular include error, got nil")
	}
}

func TestLoadConfigDenyPatterns(t *testing.T) {
	data := []byte(`
[[deny.simple]]
name = "dangerous"
commands = ["rm", "sudo"]

[[deny.regex]]
pattern = 'rm\s+-rf\s+/'
name = "rm root"
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.DenyPatterns) != 3 {
		t.Errorf("expected 3 deny patterns, got %d", len(cfg.DenyPatterns))
	}
}

// Validation tests

func TestValidateSimpleCommandsMissing(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "empty"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing commands field")
	}
	if !strings.Contains(err.Error(), "commands.simple[0]") {
		t.Errorf("error should reference commands.simple[0], got: %v", err)
	}
	if !strings.Contains(err.Error(), "\"commands\" field is required") {
		t.Errorf("error should mention commands field, got: %v", err)
	}
}

func TestValidateSimpleCommandsEmpty(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "empty"
commands = []
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for empty commands array")
	}
}

func TestValidateCommandMissing(t *testing.T) {
	data := []byte(`
[[wrappers.command]]
flags = ["-n <arg>"]
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing command field")
	}
	if !strings.Contains(err.Error(), "wrappers.command[0]") {
		t.Errorf("error should reference wrappers.command[0], got: %v", err)
	}
}

func TestValidateCommandEmpty(t *testing.T) {
	data := []byte(`
[[wrappers.command]]
command = ""
flags = ["-n <arg>"]
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for empty command field")
	}
}

func TestValidateSubcommandCommandMissing(t *testing.T) {
	data := []byte(`
[[commands.subcommand]]
subcommands = ["diff", "log"]
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing command field")
	}
	if !strings.Contains(err.Error(), "commands.subcommand[0]") {
		t.Errorf("error should reference commands.subcommand[0], got: %v", err)
	}
}

func TestValidateSubcommandSubcommandsMissing(t *testing.T) {
	data := []byte(`
[[commands.subcommand]]
command = "git"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing subcommands field")
	}
	if !strings.Contains(err.Error(), `"git"`) {
		t.Errorf("error should include command name, got: %v", err)
	}
}

func TestValidateSubcommandSubcommandsEmpty(t *testing.T) {
	data := []byte(`
[[commands.subcommand]]
command = "git"
subcommands = []
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for empty subcommands array")
	}
}

func TestValidateRegexPatternMissing(t *testing.T) {
	data := []byte(`
[[commands.regex]]
name = "empty pattern"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing pattern field")
	}
	if !strings.Contains(err.Error(), "commands.regex[0]") {
		t.Errorf("error should reference commands.regex[0], got: %v", err)
	}
}

func TestValidateRegexPatternEmpty(t *testing.T) {
	data := []byte(`
[[commands.regex]]
pattern = ""
name = "empty pattern"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for empty pattern field")
	}
}

func TestValidateDenySimpleCommandsMissing(t *testing.T) {
	data := []byte(`
[[deny.simple]]
name = "dangerous"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing commands in deny.simple")
	}
	if !strings.Contains(err.Error(), "deny.simple[0]") {
		t.Errorf("error should reference deny.simple[0], got: %v", err)
	}
}

func TestValidateDenyRegexPatternMissing(t *testing.T) {
	data := []byte(`
[[deny.regex]]
name = "dangerous"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing pattern in deny.regex")
	}
	if !strings.Contains(err.Error(), "deny.regex[0]") {
		t.Errorf("error should reference deny.regex[0], got: %v", err)
	}
}

func TestValidationErrorIncludesName(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "my custom name"
commands = []
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "my custom name") {
		t.Errorf("error should include the name field value, got: %v", err)
	}
}

func TestValidationErrorCorrectIndex(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "valid"
commands = ["ls"]

[[commands.simple]]
name = "also valid"
commands = ["cat"]

[[commands.simple]]
name = "invalid entry"
commands = []
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "commands.simple[2]") {
		t.Errorf("error should reference commands.simple[2], got: %v", err)
	}
}
