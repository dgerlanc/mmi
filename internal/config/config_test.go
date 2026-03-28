package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitErrorNilOnValidConfig(t *testing.T) {
	// Create temp directory with valid config
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	validConfig := `
[[commands.simple]]
name = "test"
commands = ["echo"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(validConfig), 0644); err != nil {
		t.Fatal(err)
	}

	Reset()
	if err := Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	if InitError() != nil {
		t.Errorf("InitError() = %v, want nil", InitError())
	}
}

func TestInitErrorOnInvalidTOML(t *testing.T) {
	// Create temp directory with invalid TOML
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	invalidConfig := `
[[commands.simple]]
name = "test"
commands = ["foo""]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(invalidConfig), 0644); err != nil {
		t.Fatal(err)
	}

	Reset()
	err := Init()
	if err == nil {
		t.Fatal("Init() should have returned an error for invalid TOML")
	}

	initErr := InitError()
	if initErr == nil {
		t.Fatal("InitError() should return non-nil error after failed Init()")
	}

	if !strings.Contains(initErr.Error(), "failed to load config") {
		t.Errorf("InitError() = %v, want error containing 'failed to load config'", initErr)
	}
}

func TestInitErrorOnMissingConfigFile(t *testing.T) {
	// Point to a directory with no config.toml
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	Reset()
	err := Init()
	if err == nil {
		t.Fatal("Init() should have returned an error for missing config file")
	}

	initErr := InitError()
	if initErr == nil {
		t.Fatal("InitError() should return non-nil error after failed Init()")
	}

	if !strings.Contains(initErr.Error(), "failed to read config.toml") {
		t.Errorf("InitError() = %v, want error containing 'failed to read config.toml'", initErr)
	}
}

func TestResetClearsInitError(t *testing.T) {
	// Create a broken config to produce an error
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`bad toml {{`), 0644); err != nil {
		t.Fatal(err)
	}

	Reset()
	Init()

	if InitError() == nil {
		t.Fatal("expected non-nil InitError before Reset")
	}

	Reset()

	if InitError() != nil {
		t.Errorf("InitError() = %v after Reset(), want nil", InitError())
	}
}

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

func TestLoadConfigSubshellDefaultsFalse(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should default to false")
	}
}

func TestLoadConfigSubshellAllowAllTrue(t *testing.T) {
	data := []byte(`
[subshell]
allow_all = true

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be true when allow_all = true")
	}
}

func TestLoadConfigSubshellAllowAllFalse(t *testing.T) {
	data := []byte(`
[subshell]
allow_all = false

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be false when allow_all = false")
	}
}

func TestLoadConfigSubshellAllowAllIncludeOverride(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[subshell]
allow_all = false

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[subshell]
allow_all = true
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be false — main config overrides included file")
	}
}

func TestLoadConfigSubshellAllowAllFromInclude(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[subshell]
allow_all = true
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	if !cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be true — inherited from included file")
	}
}

func TestLoadConfigSubshellAllowAllInvalidType(t *testing.T) {
	data := []byte(`
[subshell]
allow_all = "yes"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be false when allow_all has invalid type")
	}
}

func TestGetConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	validConfig := `
[[commands.simple]]
name = "test"
commands = ["echo"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(validConfig), 0644); err != nil {
		t.Fatal(err)
	}

	Reset()
	defer Reset()
	if err := Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	got := GetConfigPath()
	want := filepath.Join(tmpDir, "config.toml")
	if got != want {
		t.Errorf("GetConfigPath() = %q, want %q", got, want)
	}
}

func TestLoadConfigRewritesSimple(t *testing.T) {
	data := []byte(`
[[rewrites.simple]]
name = "use uv"
match = ["python", "python3"]
replace = "uv run python"
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.RewriteRules) != 2 {
		t.Fatalf("expected 2 rewrite rules, got %d", len(cfg.RewriteRules))
	}
	if cfg.RewriteRules[0].Name != "use uv" {
		t.Errorf("Name = %q, want %q", cfg.RewriteRules[0].Name, "use uv")
	}
	if cfg.RewriteRules[0].Replace != "uv run python" {
		t.Errorf("Replace = %q, want %q", cfg.RewriteRules[0].Replace, "uv run python")
	}
	if cfg.RewriteRules[0].Type != "simple" {
		t.Errorf("Type = %q, want %q", cfg.RewriteRules[0].Type, "simple")
	}
}

func TestLoadConfigRewritesRegex(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "use uv for pip"
pattern = '^pip3?\b'
replace = "uv pip"
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.RewriteRules) != 1 {
		t.Fatalf("expected 1 rewrite rule, got %d", len(cfg.RewriteRules))
	}
	if cfg.RewriteRules[0].Name != "use uv for pip" {
		t.Errorf("Name = %q, want %q", cfg.RewriteRules[0].Name, "use uv for pip")
	}
	if cfg.RewriteRules[0].Type != "regex" {
		t.Errorf("Type = %q, want %q", cfg.RewriteRules[0].Type, "regex")
	}
}

func TestLoadConfigRewritesSimpleMissingMatch(t *testing.T) {
	data := []byte(`
[[rewrites.simple]]
name = "use uv"
replace = "uv run python"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing match field")
	}
	if !strings.Contains(err.Error(), "rewrites.simple[0]") {
		t.Errorf("error should reference rewrites.simple[0], got: %v", err)
	}
	if !strings.Contains(err.Error(), "\"match\" field is required") {
		t.Errorf("error should mention match field, got: %v", err)
	}
}

func TestLoadConfigRewritesSimpleMissingReplace(t *testing.T) {
	data := []byte(`
[[rewrites.simple]]
name = "use uv"
match = ["python"]
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing replace field")
	}
	if !strings.Contains(err.Error(), "\"replace\" field is required") {
		t.Errorf("error should mention replace field, got: %v", err)
	}
}

func TestLoadConfigRewritesRegexMissingPattern(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "use uv"
replace = "uv pip"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing pattern field")
	}
	if !strings.Contains(err.Error(), "rewrites.regex[0]") {
		t.Errorf("error should reference rewrites.regex[0], got: %v", err)
	}
}

func TestLoadConfigRewritesRegexMissingReplace(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "use uv"
pattern = '^pip3?\b'
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing replace field")
	}
}

func TestLoadConfigRewritesRegexInvalidPattern(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "bad"
pattern = '[invalid'
replace = "foo"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestLoadConfigRewritesMergeIncludes(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[[rewrites.simple]]
name = "main rewrite"
match = ["python"]
replace = "uv run python"
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[[rewrites.simple]]
name = "extra rewrite"
match = ["pip"]
replace = "uv pip"
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}

	if len(cfg.RewriteRules) != 2 {
		t.Errorf("expected 2 rewrite rules after merge, got %d", len(cfg.RewriteRules))
	}
}

func TestLoadConfigUnmatchedDefaultsToAsk(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "ask" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "ask")
	}
}

func TestLoadConfigUnmatchedPassthrough(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "passthrough" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "passthrough")
	}
}

func TestLoadConfigUnmatchedReject(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "reject"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "reject" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "reject")
	}
}

func TestLoadConfigUnmatchedAskExplicit(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "ask"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "ask" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "ask")
	}
}

func TestLoadConfigUnmatchedInvalidValue(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "foo"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for invalid unmatched value")
	}
	if !strings.Contains(err.Error(), "unmatched") {
		t.Errorf("error should mention 'unmatched', got: %v", err)
	}
}

func TestLoadConfigUnmatchedIncludeOverride(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[defaults]
unmatched = "reject"
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	// Main config is processed after includes, so main's value wins
	if cfg.Unmatched != "passthrough" {
		t.Errorf("Unmatched = %q, want %q (main config should override include)", cfg.Unmatched, "passthrough")
	}
}

func TestLoadConfigUnmatchedFromInclude(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[defaults]
unmatched = "passthrough"
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	// Main config omits [defaults], so include's value survives.
	// Same semantics as SubshellAllowAll: unconditional assignment, last value wins.
	if cfg.Unmatched != "passthrough" {
		t.Errorf("Unmatched = %q, want %q (inherited from include)", cfg.Unmatched, "passthrough")
	}
}

func TestGetConfigPathAfterReset(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	validConfig := `
[[commands.simple]]
name = "test"
commands = ["echo"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(validConfig), 0644); err != nil {
		t.Fatal(err)
	}

	Reset()
	Init()
	Reset()

	if got := GetConfigPath(); got != "" {
		t.Errorf("GetConfigPath() after Reset() = %q, want empty string", got)
	}
}
