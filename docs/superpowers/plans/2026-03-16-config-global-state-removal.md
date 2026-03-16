# Remove Config Global State Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove global configuration state from `internal/config` and thread config explicitly through all consumers via function parameters and closures.

**Architecture:** Replace the `config.Init()` / `config.Get()` singleton with a `config.Load()` constructor that returns `(*Config, string, error)`. The `cmd` layer creates config in `PersistentPreRunE` and passes it to subcommands via closure scope. The `hook` layer receives config as a function parameter.

**Tech Stack:** Go, Cobra CLI framework, TOML config parsing

**Spec:** `docs/superpowers/specs/2026-03-16-config-global-state-removal-design.md`

---

## Chunk 1: Core config and hook changes

### Task 1: Add `config.Load()` constructor

**Files:**
- Modify: `internal/config/config.go:32-41` (global vars), `369-446` (Init/Get/Reset/etc.)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write tests for `Load()`**

Replace the first four tests in `config_test.go` (which test `Init`/`InitError`/`Reset`) with tests for `Load()`. These tests exercise the same error paths but against the new API:

```go
func TestLoadValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	validConfig := `
[[commands.simple]]
name = "test"
commands = ["echo"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(validConfig), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, cfgPath, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	if cfgPath == "" {
		t.Error("Load() returned empty config path for existing file")
	}
	if len(cfg.SafeCommands) != 1 {
		t.Errorf("expected 1 safe command, got %d", len(cfg.SafeCommands))
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	invalidConfig := `
[[commands.simple]]
name = "test"
commands = ["foo""]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(invalidConfig), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, cfgPath, err := Load()
	if err == nil {
		t.Fatal("Load() should have returned an error for invalid TOML")
	}
	if cfg == nil {
		t.Fatal("Load() should return non-nil defaults even on error")
	}
	if cfgPath == "" {
		t.Error("Load() should return config path even on parse error")
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	cfg, cfgPath, err := Load()
	if err != nil {
		t.Fatalf("Load() should not error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() should return non-nil defaults")
	}
	if cfgPath != "" {
		t.Errorf("Load() should return empty path for missing file, got: %q", cfgPath)
	}
}

func TestLoadReturnsDefaultsOnEveryError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	// Write a file that's not valid TOML
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(`bad toml {{`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if cfg == nil {
		t.Fatal("Load() must always return non-nil config")
	}
	// Defaults should be empty (deny-all)
	if len(cfg.SafeCommands) != 0 {
		t.Errorf("expected 0 safe commands in defaults, got %d", len(cfg.SafeCommands))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test ./internal/config/ -run "TestLoad(Valid|Invalid|Missing|ReturnsDefaults)" -v`
Expected: FAIL — `Load` is not defined

- [ ] **Step 3: Implement `Load()` and remove global state**

In `internal/config/config.go`:

1. Remove the global variables block (lines 32-41):
```go
// DELETE:
var (
	globalConfig     *Config
	configInitialized bool
	globalInitError  error
	globalConfigPath string
)
```

2. Remove functions: `Init()` (lines 369-416), `Get()` (lines 418-425), `InitError()` (lines 427-432), `GetConfigPath()` (lines 434-438), `Reset()` (lines 440-446).

3. Add `Load()`:
```go
// Load reads config from the standard config directory (respecting MMI_CONFIG env var).
// Returns the loaded Config, the resolved config file path, and any error.
// Always returns a non-nil Config (embedded defaults on error).
func Load() (*Config, string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return loadEmbeddedDefaults(), "", fmt.Errorf("resolving config directory: %w", err)
	}
	configPath := filepath.Join(configDir, constants.ConfigFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return loadEmbeddedDefaults(), "", nil
		}
		return loadEmbeddedDefaults(), "", err
	}

	cfg, err := LoadConfigWithDir(data, configDir)
	if err != nil {
		return loadEmbeddedDefaults(), configPath, err
	}

	logger.Debug("config loaded successfully",
		"path", configPath,
		"safe_commands", len(cfg.SafeCommands),
		"wrappers", len(cfg.WrapperPatterns),
		"deny_patterns", len(cfg.DenyPatterns),
		"subshell_allow_all", cfg.SubshellAllowAll,
	)
	return cfg, configPath, nil
}
```

4. Remove the `"github.com/dgerlanc/mmi/internal/config"` import from itself if present (it's internal, but verify no circular ref issues from removing `Init`).

- [ ] **Step 4: Remove old tests for Init/InitError/Reset/GetConfigPath**

Delete from `internal/config/config_test.go`:
- `TestInitErrorNilOnValidConfig`
- `TestInitErrorOnInvalidTOML`
- `TestInitErrorOnMissingConfigFile`
- `TestResetClearsInitError`
- `TestGetConfigPath` (calls `Init()`, `GetConfigPath()`)
- `TestGetConfigPathAfterReset` (calls `Init()`, `Reset()`, `GetConfigPath()`)

- [ ] **Step 5: Run config tests**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test ./internal/config/ -v`
Expected: All `TestLoad*` tests pass. Other tests (`TestLoadConfig`, `TestLoadConfigWithIncludes`, etc.) should still pass as they test pure functions.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "refactor: replace config global state with Load() constructor

Remove globalConfig, configInitialized, globalInitError, globalConfigPath.
Remove Init(), Get(), InitError(), GetConfigPath(), Reset().
Add Load() that returns (*Config, string, error) with embedded defaults
on all error paths."
```

---

### Task 2: Update `hook.ProcessWithResult` and `logAudit` signatures

**Files:**
- Modify: `internal/hook/hook.go:167-170` (Process), `174-320` (ProcessWithResult), `367-387` (logAudit)

- [ ] **Step 1: Update `ProcessWithResult` signature**

Change signature from `ProcessWithResult(r io.Reader) Result` to:
```go
func ProcessWithResult(r io.Reader, cfg *config.Config, cfgPath string, cfgErr error) Result
```

Inside the function body:
- Remove `cfg := config.Get()` (line 202)
- The `cfg` parameter is now used directly

- [ ] **Step 2: Update `logAudit` signature and callers**

Change `logAudit` from:
```go
func logAudit(command string, approved bool, segments []audit.Segment, durationMs float64, sessionID, toolUseID, cwd, rawInput, rawOutput string)
```
to:
```go
func logAudit(command string, approved bool, segments []audit.Segment, durationMs float64, sessionID, toolUseID, cwd, rawInput, rawOutput, configPath, configError string)
```

Inside `logAudit`, replace:
```go
configPath := config.GetConfigPath()
var configError string
if err := config.InitError(); err != nil {
    configError = err.Error()
}
```
with just using the `configPath` and `configError` parameters directly.

In `ProcessWithResult`, compute `configError` once at the top and pass to all `logAudit` calls:
```go
var configError string
if cfgErr != nil {
    configError = cfgErr.Error()
}
```

Update all 3 `logAudit` call sites within `ProcessWithResult` (lines 214, 312, 318) to pass `cfgPath, configError` as the last two arguments.

- [ ] **Step 3: Update `Process` wrapper**

Change from:
```go
func Process(r io.Reader) (approved bool, reason string) {
	result := ProcessWithResult(r)
	return result.Approved, result.Reason
}
```
to:
```go
func Process(r io.Reader, cfg *config.Config, cfgPath string, cfgErr error) (approved bool, reason string) {
	result := ProcessWithResult(r, cfg, cfgPath, cfgErr)
	return result.Approved, result.Reason
}
```

- [ ] **Step 4: Verify config import is still needed**

The `config` import in `hook.go` is still required for the `*config.Config` type reference in function signatures. Do NOT remove it. Just verify there are no remaining calls to `config.Get()`, `config.GetConfigPath()`, or `config.InitError()`.

- [ ] **Step 5: Verify hook package compiles**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go build ./internal/hook/`
Expected: Compile success (tests will fail because callers haven't been updated yet)

- [ ] **Step 6: Commit**

```bash
git add internal/hook/hook.go
git commit -m "refactor: pass config explicitly to ProcessWithResult and logAudit

ProcessWithResult now accepts (*config.Config, cfgPath, cfgErr) instead
of calling config.Get()/GetConfigPath()/InitError() internally.
Process wrapper updated to match."
```

---

### Task 3: Update `cmd` package — builder pattern

**Files:**
- Modify: `cmd/root.go` (full rewrite), `cmd/run.go` (delete or gut), `cmd/completion.go`, `cmd/validate.go`, `cmd/init.go`

- [ ] **Step 1: Rewrite `cmd/root.go`**

Replace entire file with:

```go
// Package cmd implements the CLI commands for mmi.
package cmd

import (
	"fmt"
	"os"

	"github.com/dgerlanc/mmi/internal/audit"
	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/hook"
	"github.com/dgerlanc/mmi/internal/logger"
	"github.com/spf13/cobra"
)

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return buildRootCmd().Execute()
}

func buildRootCmd() *cobra.Command {
	var (
		verbose    bool
		dryRun     bool
		noAuditLog bool
		cfg        *config.Config
		cfgPath    string
		cfgErr     error
	)

	rootCmd := &cobra.Command{
		Use:   "mmi",
		Short: "Mother May I? - Claude Code Bash command approval hook",
		Long: `MMI (Mother May I?) is a PreToolUse hook for Claude Code that
approves or rejects Bash commands based on configurable patterns.

When called without arguments, it reads a JSON command from stdin and outputs
an the approval decision to stdout as JSON.

Usage in ~/.claude/settings.json:
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command", "command": "mmi"}]
    }]
  }`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logger.Init(logger.Options{Verbose: verbose})
			cfg, cfgPath, cfgErr = config.Load()
			audit.Init("", noAuditLog)
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			result := hook.ProcessWithResult(os.Stdin, cfg, cfgPath, cfgErr)

			if dryRun {
				if result.Approved {
					fmt.Fprintf(os.Stderr, "APPROVED: %s (reason: %s)\n", result.Command, result.Reason)
				} else if result.Command != "" {
					fmt.Fprintf(os.Stderr, "REJECTED: %s\n", result.Command)
				} else {
					fmt.Fprintf(os.Stderr, "REJECTED: (no command parsed)\n")
				}
				return
			}

			fmt.Print(result.Output)
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output (debug logging)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Test command approval without JSON output")
	rootCmd.PersistentFlags().BoolVar(&noAuditLog, "no-audit-log", false, "Disable audit logging")

	rootCmd.AddCommand(buildValidateCmd(&cfg, &cfgPath, &cfgErr))
	rootCmd.AddCommand(buildInitCmd())
	rootCmd.AddCommand(buildCompletionCmd(rootCmd))

	return rootCmd
}
```

- [ ] **Step 2: Delete `cmd/run.go`**

The `runHook` logic is now inlined in `buildRootCmd`'s `Run` closure. Delete the entire file.

- [ ] **Step 3: Rewrite `cmd/completion.go`**

Replace with:

```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func buildCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for mmi.

To load completions:

Bash:
  $ source <(mmi completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ mmi completion bash > /etc/bash_completion.d/mmi
  # macOS:
  $ mmi completion bash > $(brew --prefix)/etc/bash_completion.d/mmi

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ mmi completion zsh > "${fpath[1]}/_mmi"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ mmi completion fish | source
  # To load completions for each session, execute once:
  $ mmi completion fish > ~/.config/fish/completions/mmi.fish

PowerShell:
  PS> mmi completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> mmi completion powershell > mmi.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
}
```

- [ ] **Step 4: Rewrite `cmd/validate.go`**

Replace with:

```go
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

			fmt.Println("Configuration valid!")
			fmt.Println()

			fmt.Printf("Subshell allow all: %v\n", (*cfg).SubshellAllowAll)
			fmt.Println()

			fmt.Printf("Deny patterns: %d\n", len((*cfg).DenyPatterns))
			for _, p := range (*cfg).DenyPatterns {
				fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
			}
			fmt.Println()

			fmt.Printf("Wrapper patterns: %d\n", len((*cfg).WrapperPatterns))
			for _, p := range (*cfg).WrapperPatterns {
				fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
			}
			fmt.Println()

			fmt.Printf("Safe command patterns: %d\n", len((*cfg).SafeCommands))
			for _, p := range (*cfg).SafeCommands {
				fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
			}

			return nil
		},
	}
}
```

- [ ] **Step 5: Rewrite `cmd/init.go`**

Replace the `init()` function and package-level variables. Keep `runInit` and the rest of the file, but change the top to:

Replace lines 15-42 (global vars + `initCmd` var + `init()`) with:

```go
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
```

Then update `runInit` signature from:
```go
func runInit(cmd *cobra.Command, args []string) error {
```
to:
```go
func runInit(cmd *cobra.Command, args []string, force, configOnly bool, claudeSettings string) error {
```

And replace all references to `initForce` → `force`, `initConfigOnly` → `configOnly`, `initClaudeSettings` → `claudeSettings` inside `runInit`.

Also update `configureClaudeSettings` to accept `claudeSettings string` as a parameter (currently reads package-level `initClaudeSettings` indirectly via `getClaudeSettingsPath()`):

```go
func configureClaudeSettings(claudeSettings string) error {
```

And update `getClaudeSettingsPath` to accept `claudeSettings string`:
```go
func getClaudeSettingsPath(claudeSettings string) (string, error) {
	if claudeSettings != "" {
		return claudeSettings, nil
	}
	// ... rest unchanged
}
```

Update the call chain: `runInit` → `configureClaudeSettings(claudeSettings)` → `getClaudeSettingsPath(claudeSettings)`.

Remove the three package-level vars: `initForce`, `initConfigOnly`, `initClaudeSettings`.

- [ ] **Step 6: Verify cmd package compiles**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go build ./cmd/`
Expected: Compile errors from test files referencing old APIs — that's expected. The package itself should compile.

Note: Use `go build` not `go test` since tests still reference old APIs.

- [ ] **Step 7: Commit**

```bash
git add cmd/root.go cmd/completion.go cmd/validate.go cmd/init.go
git rm cmd/run.go
git commit -m "refactor: replace cmd init() globals with builder pattern

buildRootCmd() creates the command tree with closures. Config is loaded
in PersistentPreRunE and flows to subcommands via closure scope.
Remove runHook (inlined), IsVerbose, IsDryRun, package-level flags."
```

---

## Chunk 2: Test updates

### Task 4: Update `internal/testutil`

**Files:**
- Modify: `internal/testutil/testutil.go`

- [ ] **Step 1: Rewrite `SetupTestConfig` to return `*Config`**

Replace entire file:

```go
// Package testutil provides shared test utilities for mmi tests.
package testutil

import (
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
)

// LoadTestConfig parses config content and returns a *Config for testing.
// Fails the test on parse error.
func LoadTestConfig(t *testing.T, configContent string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig([]byte(configContent))
	if err != nil {
		t.Fatalf("LoadTestConfig: %v", err)
	}
	return cfg
}

// MinimalTestConfig is a minimal config for testing.
const MinimalTestConfig = `
[[commands.simple]]
name = "safe"
commands = ["ls", "cat", "echo"]

[[deny.simple]]
name = "dangerous"
commands = ["rm"]
`
```

- [ ] **Step 2: Commit**

```bash
git add internal/testutil/testutil.go
git commit -m "refactor: simplify testutil to return *Config directly

Replace SetupTestConfig (env var + Reset/Init dance) with LoadTestConfig
that parses config content and returns a value."
```

---

### Task 5: Update `cmd` test files

**Files:**
- Modify: `cmd/root_test.go`, `cmd/run_test.go`, `cmd/validate_test.go`, `cmd/init_test.go`

- [ ] **Step 1: Rewrite `cmd/root_test.go`**

Delete `resetGlobalState`, `TestIsVerbose`, `TestIsDryRun`, `TestInitAppWithEnvConfig`, `TestRootCmdFlags`.

Rewrite `TestRootCmdHasExpectedSubcommands` and `TestRootCmdUsageContainsDescription` to use `buildRootCmd()`:

```go
package cmd

import (
	"testing"
)

func TestRootCmdHasExpectedSubcommands(t *testing.T) {
	rootCmd := buildRootCmd()
	subcommands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expected := []string{"validate", "init", "completion"}
	for _, name := range expected {
		if !subcommands[name] {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestRootCmdUsageContainsDescription(t *testing.T) {
	rootCmd := buildRootCmd()
	if rootCmd.Short == "" {
		t.Error("root command should have a short description")
	}
	if rootCmd.Long == "" {
		t.Error("root command should have a long description")
	}
}
```

- [ ] **Step 2: Rewrite `cmd/run_test.go`**

Delete `setupTestConfig`. Update all tests to use `buildRootCmd()` with config set up via temp dirs and `MMI_CONFIG` env var. Each test creates its own root command:

The pattern for each test becomes:
```go
func TestRunHookDryRunApproved(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)
	// write test config to tmpDir...

	rootCmd := buildRootCmd()
	rootCmd.SetArgs([]string{"--dry-run"})
	rootCmd.SetIn(strings.NewReader(jsonInput))
	// capture stderr...
	rootCmd.Execute()
	// assert stderr output...
}
```

Follow the existing test patterns but replace `setupTestConfig(t)` + direct `runHook` calls with `buildRootCmd()` + `Execute()`. The tests should exercise the full command path since `runHook` is now inlined.

- [ ] **Step 3: Rewrite `cmd/validate_test.go`**

Tests currently call `runValidate(cmd, []string{})` directly and reference `validateCmd`. Both are removed. Replace every test to use `buildRootCmd()` with `SetArgs([]string{"validate"})`:

```go
rootCmd := buildRootCmd()
rootCmd.SetArgs([]string{"validate"})
```

The `t.Setenv("MMI_CONFIG", tmpDir)` pattern provides the config. Tests that capture stdout should use `rootCmd.SetOut(&buf)` instead of `os.Pipe`.

Specifically rewrite: `TestRunValidateWithValidConfig`, `TestRunValidateShowsPatternCounts`, `TestRunValidateShowsPatternNames`, `TestRunValidateWithInvalidConfig`, `TestRunValidateWithMissingConfig`, `TestValidateCmdUsage` (references `validateCmd`), `TestRunValidateShowsSubshellAllowAll`, `TestRunValidateShowsSubshellAllowAllFalse`, `TestRunValidateWithEmptyConfig`.

- [ ] **Step 4: Rewrite `cmd/init_test.go`**

Tests currently call `runInit(cmd, []string{})` directly, set package-level flags (`initForce`, `initConfigOnly`, `initClaudeSettings`), reference `initCmd`, and call `resetGlobalState()`. All of these are removed.

Replace every test to use `buildRootCmd()` with `SetArgs`:

```go
// For flags, pass them as command-line args:
rootCmd := buildRootCmd()
rootCmd.SetArgs([]string{"init", "--force"})
rootCmd.SetArgs([]string{"init", "--config-only"})
rootCmd.SetArgs([]string{"init", "--claude-settings", "/path/to/settings.json"})
```

Tests that reference `initCmd` directly (e.g., `TestInitCmdHasForceFlag`, `TestInitCmdUsage`, `TestInitCmdHasConfigOnlyFlag`, `TestInitCmdHasClaudeSettingsFlag`) should find the "init" subcommand from `buildRootCmd().Commands()`.

Remove all `resetGlobalState()` calls and direct `initForce`/`initConfigOnly`/`initClaudeSettings` assignments.

- [ ] **Step 5: Run cmd tests**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test ./cmd/ -v`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add cmd/root_test.go cmd/run_test.go cmd/validate_test.go cmd/init_test.go
git commit -m "test: update cmd tests to use buildRootCmd() builder

Each test creates its own command tree — no shared global state.
Delete resetGlobalState, setupTestConfig, TestIsVerbose, TestIsDryRun."
```

---

### Task 6: Update `main_test.go`

**Files:**
- Modify: `main_test.go`

- [ ] **Step 1: Replace `TestMain` with a helper that loads config**

Remove the `TestMain` function. Replace with a package-level helper:

```go
// loadTestConfig parses the testConfig TOML and returns *config.Config.
func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig([]byte(testConfig))
	if err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}
	return cfg
}
```

- [ ] **Step 2: Update `TestProcess`**

Change:
```go
approved, reason := hook.Process(strings.NewReader(tt.input))
```
to:
```go
cfg := loadTestConfig(t)
approved, reason := hook.Process(strings.NewReader(tt.input), cfg, "", nil)
```

Move `cfg` outside the loop if you prefer, since all subtests use the same config.

- [ ] **Step 3: Update `TestStripWrappers` and `TestCheckSafe`/`TestCheckSafeUnsafe`**

Replace `cfg := config.Get()` with `cfg := loadTestConfig(t)`.

- [ ] **Step 3b: Update `TestInitConfig` and `TestConfigCustomization`**

These tests call `config.Reset()`, `config.Init()`, and `config.Get()` — all removed functions.

`TestInitConfig` (line 582): Replace with a test that calls `config.Load()` with a temp dir containing no config file and verifies it returns empty defaults:
```go
func TestInitConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MMI_CONFIG", tmpDir)

	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("Load() should succeed with missing config: %v", err)
	}
	if len(cfg.WrapperPatterns) != 0 {
		t.Error("WrapperPatterns should be empty when no config file exists")
	}
	if len(cfg.SafeCommands) != 0 {
		t.Error("SafeCommands should be empty when no config file exists")
	}
	// Config file should NOT be auto-created
	if _, err := os.Stat(filepath.Join(tmpDir, "config.toml")); err == nil {
		t.Error("config.toml should not be auto-created")
	}
}
```

`TestConfigCustomization` (line 617): Replace `config.Reset()` / `config.Init()` / `config.Get()` with `config.Load()`:
```go
func TestConfigCustomization(t *testing.T) {
	tmpDir := t.TempDir()
	configToml := []byte(`
[[wrappers.regex]]
pattern = "^custom\\s+"
name = "custom"

[[commands.subcommand]]
command = "mycommand"
subcommands = ["arg"]
`)
	os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)
	t.Setenv("MMI_CONFIG", tmpDir)

	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	core, wrappers := hook.StripWrappers("custom mycommand arg", cfg.WrapperPatterns)
	if len(wrappers) != 1 || wrappers[0] != "custom" {
		t.Errorf("Custom wrapper not stripped: %v", wrappers)
	}
	if result := hook.CheckSafe(core, cfg.SafeCommands); result.Name != "mycommand" {
		t.Errorf("Custom command not recognized: %q (got %q)", core, result.Name)
	}
}
```

- [ ] **Step 4: Update integration test `runMmi`**

The `runMmi` helper builds a binary and runs it. It currently inherits `MMI_CONFIG` from the env (set by `TestMain`). Since `TestMain` is removed, each integration test must set up its own temp config dir:

```go
func runMmi(t *testing.T, input string) (string, int) {
	t.Helper()

	// Set up config in temp dir
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", filepath.Join(tmpDir, "mmi_test_binary"), ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build: %v", err)
	}

	cmd = exec.Command(filepath.Join(tmpDir, "mmi_test_binary"))
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "MMI_CONFIG="+tmpDir)
	// ... rest stays the same
}
```

- [ ] **Step 5: Remove `config` import if no longer needed**

If all `config.Get()` calls are replaced with `loadTestConfig`, the `config` import may only be needed for `config.LoadConfig`. Check and update imports.

- [ ] **Step 6: Run main_test**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test -v -count=1`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add main_test.go
git commit -m "test: remove TestMain, use explicit config in each test

Replace global config.Init() with loadTestConfig() helper that returns
*Config directly. Integration tests set up their own temp config dirs."
```

---

### Task 7: Update `benchmark_test.go` and `fuzz_test.go`

**Files:**
- Modify: `benchmark_test.go`, `fuzz_test.go`

- [ ] **Step 1: Update `benchmark_test.go`**

Replace `config.Get()` calls with `config.LoadConfig([]byte(testConfig))`:

```go
func BenchmarkProcess(b *testing.B) {
	cfg, err := config.LoadConfig([]byte(testConfig))
	if err != nil {
		b.Fatalf("failed to load config: %v", err)
	}

	// ... benchmarks stay the same ...
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = hook.ProcessWithResult(strings.NewReader(bm.input), cfg, "", nil)
			}
		})
	}
}
```

Apply same pattern to `BenchmarkStripWrappers`, `BenchmarkCheckSafe`, `BenchmarkCheckDeny` — construct config once before the benchmark loop.

- [ ] **Step 2: Update `fuzz_test.go`**

Replace `getTestConfig()` with inline config loading:

```go
func getTestConfig(t testing.TB) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig([]byte(testConfig))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	return cfg
}
```

Update `FuzzProcess`:
```go
func FuzzProcess(f *testing.F) {
	// ... seeds ...
	cfg := getTestConfig(f)
	f.Fuzz(func(t *testing.T, input string) {
		_ = hook.ProcessWithResult(strings.NewReader(input), cfg, "", nil)
	})
}
```

Update `FuzzStripWrappers`, `FuzzCheckSafe`, `FuzzCheckDeny` similarly — call `getTestConfig(f)` once before `f.Fuzz`, capture in closure.

- [ ] **Step 3: Run benchmarks and fuzz tests**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test -bench=. -benchtime=100ms -count=1`
Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test -fuzz=FuzzProcess -fuzztime=5s`
Expected: No panics, benchmarks complete

- [ ] **Step 4: Commit**

```bash
git add benchmark_test.go fuzz_test.go
git commit -m "test: update benchmark and fuzz tests for explicit config

Construct config once before hot loops. No more config.Get() calls."
```

---

### Task 8: Update `internal/hook` tests

**Files:**
- Modify: `internal/hook/hook_test.go` (~1770 lines, ~30 `ProcessWithResult` calls)

This is a large test file. The changes are mechanical but extensive.

- [ ] **Step 1: Update all `ProcessWithResult` and `Process` calls**

Search for all calls to `ProcessWithResult(` and `Process(` in `internal/hook/hook_test.go`. Each call currently passes only `io.Reader`. Add `cfg, "", nil` as the additional three arguments.

The test file already constructs config via `testutil.SetupTestConfig` or `config.Reset()`/`config.Init()`. Replace those patterns:

**Pattern A** — tests using `testutil.SetupTestConfig(t, content)`:
Replace with `cfg := testutil.LoadTestConfig(t, content)` and pass `cfg` to `ProcessWithResult`.

**Pattern B** — tests using `config.Reset()` + `config.Init()` with env var setup:
Replace with `cfg, _, _ := config.Load()` after setting up the temp dir, or use `config.LoadConfig([]byte(content))` directly.

**Pattern C** — tests that intentionally test with broken/missing config (e.g., `TestProcessWithResultAuditConfigErrorOnInvalidConfig`):
Use `config.Load()` which returns defaults + error, then pass the error as `cfgErr`:
```go
cfg, cfgPath, cfgErr := config.Load()
result := ProcessWithResult(strings.NewReader(input), cfg, cfgPath, cfgErr)
```

- [ ] **Step 2: Remove all `config.Reset()` and `config.Init()` calls**

Search and remove every `config.Reset()` and `config.Init()` call. Remove `testutil.SetupTestConfig` cleanup functions (no longer needed since there's no global state to clean up).

- [ ] **Step 3: Update imports**

Remove `config` import if no longer needed (only needed if tests call `config.Load()` or `config.LoadConfig()`). Keep it if tests construct config via `config.LoadConfig()`.

- [ ] **Step 4: Run hook tests**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test ./internal/hook/ -v -count=1`
Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/hook/hook_test.go
git commit -m "test: update hook tests for explicit config parameter

Update ~30 ProcessWithResult calls to pass config, cfgPath, cfgErr.
Remove config.Reset()/config.Init() dance from test setup."
```

---

### Task 9: Add `t.Parallel()` to config and hook tests

**Files:**
- Modify: `internal/config/config_test.go`, `main_test.go`

- [ ] **Step 1: Add `t.Parallel()` to config tests**

Add `t.Parallel()` as the first line of each test function in `internal/config/config_test.go` that does NOT use `t.Setenv` (since `t.Setenv` is incompatible with `t.Parallel` in Go). For tests using `t.Setenv`, they already get automatic cleanup so parallelism is safe if each uses its own temp dir — but Go forbids the combination. Leave those sequential.

For tests that use `LoadConfig`/`LoadConfigWithDir` with inline data (no env vars), add `t.Parallel()`.

- [ ] **Step 2: Add `t.Parallel()` to main_test.go tests**

Add `t.Parallel()` to tests that use `loadTestConfig(t)` and don't depend on env vars or shared state. This includes `TestProcess`, `TestFormatApproval`, `TestSplitCommandChain`, `TestStripWrappers`, `TestCheckSafe`, `TestCheckSafeUnsafe`.

Skip integration tests (`TestIntegration*`) since they build binaries and use temp dirs with env vars.

- [ ] **Step 3: Run all tests to verify no races**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test ./... -race -count=1`
Expected: All pass, no data races

- [ ] **Step 4: Commit**

```bash
git add internal/config/config_test.go main_test.go
git commit -m "test: enable t.Parallel() for config and hook tests

Now possible because tests no longer share global config state."
```

---

### Task 10: Final verification

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go test ./... -race -count=1 -v`
Expected: All tests pass

- [ ] **Step 2: Run go vet**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && go vet ./...`
Expected: No issues

- [ ] **Step 3: Verify no remaining references to removed functions**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && grep -rn 'config\.Get()\|config\.Reset()\|config\.Init()\|config\.InitError()\|config\.GetConfigPath()\|config\.ConfigPath()' --include='*.go'`
Expected: No matches

- [ ] **Step 4: Verify no remaining global config vars**

Run: `cd /Users/dgerlanc/code/mmi/.claude/worktrees/config-global-state && grep -rn 'globalConfig\|configInitialized\|globalInitError\|globalConfigPath' --include='*.go'`
Expected: No matches
