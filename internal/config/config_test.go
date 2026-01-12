package config

import (
	"os"
	"path/filepath"
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
