# Remove Global Configuration State

## Problem

The `internal/config` package uses package-level global variables (`globalConfig`, `configInitialized`, `globalInitError`, `globalConfigPath`) accessed via `Get()` and managed via `Init()` / `Reset()`. This pattern:

- Prevents parallel test execution (`t.Parallel()`) because tests share and mutate global state
- Forces a `Reset()` / `Init()` dance in test setup/teardown
- Hides function dependencies — callers like `hook.ProcessWithResult()` import config and call `Get()` internally, making the dependency invisible in the function signature
- Couples the `cmd` package to config's global state via `cobra.OnInitialize`

## Scope

**Phase 1 (this spec):** Remove global state from `internal/config` and update all consumers.

**Future phases (out of scope):** Apply the same pattern to `internal/audit`, `internal/logger`, and `cmd` flag globals.

## Design

### Approach: Constructor + Closures

Replace the global singleton with a pure constructor that returns a value. Thread the config through the application via function parameters and closure scope.

### `internal/config` Package

**Remove:**
- Global variables: `globalConfig`, `configInitialized`, `globalInitError`, `globalConfigPath`
- Functions: `Get()`, `Reset()`, `Init()`, `InitError()`, `ConfigPath()`

**Add:**
```go
// Load reads config from the standard config directory (respecting MMI_CONFIG env var).
// Returns the loaded Config and the resolved config file path.
func Load() (*Config, string, error) {
    configDir, err := GetConfigDir()
    if err != nil {
        // Always return usable defaults so callers never get nil config.
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
        // Return embedded defaults on parse error (deny-all) so callers always
        // get a usable config. Matches current Init() fallback behavior.
        return loadEmbeddedDefaults(), configPath, err
    }
    return cfg, configPath, nil
}
```

Note: `GetConfigDir()` returns `(string, error)` — the error case (from `os.UserHomeDir()`) is propagated.

**Keep unchanged:**
- `LoadConfig(data []byte) (*Config, error)` — pure function, no global state
- `LoadConfigWithDir(data []byte, configDir string) (*Config, error)` — pure function
- `GetConfigDir() (string, error)` — reads env var, no mutation
- `EnsureConfigFiles()` and `GetDefaultConfig()` — no global state dependency
- `Config` struct — unchanged

### `internal/hook` Package

**Change `ProcessWithResult` to accept config path and error alongside `*config.Config`:**

The current signature is `ProcessWithResult(r io.Reader) Result`. It reads JSON from stdin, parses it, extracts the command, then calls `config.Get()` to get patterns and `config.GetConfigPath()` / `config.InitError()` for audit logging.

New signature:
```go
func ProcessWithResult(r io.Reader, cfg *config.Config, cfgPath string, cfgErr error) Result
```

- `cfg` replaces the `config.Get()` call
- `cfgPath` and `cfgErr` replace `config.GetConfigPath()` and `config.InitError()` calls inside `logAudit`
- The `io.Reader` parameter and all JSON parsing stay exactly as they are

**Change `Process` wrapper to match:**
```go
func Process(r io.Reader, cfg *config.Config, cfgPath string, cfgErr error) (approved bool, reason string) {
    result := ProcessWithResult(r, cfg, cfgPath, cfgErr)
    return result.Approved, result.Reason
}
```

**Update `logAudit` to accept config path and error as parameters:**

```go
func logAudit(command string, approved bool, segments []audit.Segment, durationMs float64,
    sessionID, toolUseID, cwd, rawInput, rawOutput, configPath, configError string)
```

`ProcessWithResult` converts `cfgErr` to a string (or empty string if nil) before passing to `logAudit`.

`StripWrappers` and `CheckSafe` already accept pattern slices as parameters — no changes.

### `cmd` Package

**Replace `init()` + global flags with a builder function.**

All `init()` functions in the cmd package (`root.go`, `init.go`, `completion.go`, `validate.go`) are removed. Subcommands are wired up explicitly in builder functions.

```go
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
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            logger.Init(logger.Options{Verbose: verbose})

            cfg, cfgPath, cfgErr = config.Load()
            // cfgErr is stored, not returned — matches current behavior where
            // config parse errors are logged to audit but don't block execution

            // Always call audit.Init, matching current behavior.
            // The noAuditLog flag disables logging inside Init.
            audit.Init("", noAuditLog)
            return nil
        },
        // Default command: process stdin for command approval
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

    rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
    rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Test command approval without JSON output")
    rootCmd.PersistentFlags().BoolVar(&noAuditLog, "no-audit-log", false, "Disable audit logging")

    rootCmd.AddCommand(buildValidateCmd(&cfg, &cfgPath, &cfgErr))
    rootCmd.AddCommand(buildInitCmd())
    rootCmd.AddCommand(buildCompletionCmd(rootCmd))

    return rootCmd
}
```

The `runHook` function in `cmd/run.go` is removed. Its logic is inlined into the root command's `Run` closure, which has direct access to `cfg`, `cfgPath`, `cfgErr`, and `dryRun` from the enclosing scope.

**`completion.go`:** The current code references the package-level `rootCmd` in its `RunE` (for `GenBashCompletion` etc.) and uses `init()` to add itself. This becomes `buildCompletionCmd(rootCmd *cobra.Command)` which receives the root command as a parameter and returns the completion command.

**`validate.go`:** `buildValidateCmd` receives pointers to `cfg`, `cfgPath`, and `cfgErr` so it can display config status and any parse errors (replacing `config.Get()` and `config.InitError()`).

**`init.go`:** `buildInitCmd()` manages its own local flags (`force`, `configOnly`, `claudeSettings`). Does not need config since it creates it.

**Remove `IsVerbose()` and `IsDryRun()`** — these exported functions access the package-level globals being removed. They are only used in `cmd` package tests (`root_test.go`), not by any external package. The tests that exercise them are deleted.

**Keep `Execute()` as the public entry point** — it internally calls `buildRootCmd().Execute()`, keeping the builder as an unexported implementation detail. This preserves the current `main.go` pattern:

```go
// cmd/root.go
func Execute() error {
    return buildRootCmd().Execute()
}
```

`main.go` is unchanged:
```go
func main() {
    if err := cmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

**Note on `mmi init`:** `PersistentPreRunE` calls `config.Load()` unconditionally, including for `mmi init`. On first run (before a config file exists), `Load()` returns embedded defaults with an empty path — this matches current behavior where `config.Init()` runs unconditionally and falls back to defaults.

### Test Changes

**`internal/config` tests:**
- Remove all `Reset()` and `Init()` calls
- Tests use `LoadConfig()` / `LoadConfigWithDir()` directly — these are pure functions that return values
- No global state to manage

**`internal/hook` tests:**
- Construct `*config.Config` literals inline and pass to `ProcessWithResult` along with an `io.Reader`:
```go
cfg := &config.Config{
    SafeCommands:    []patterns.Pattern{...},
    WrapperPatterns: []patterns.Pattern{...},
}
result := hook.ProcessWithResult(strings.NewReader(jsonInput), cfg, "", nil)
```

**`main_test.go`:**
- Remove `TestMain` that sets up global config via env vars and `config.Init()`
- Each test calls `config.LoadConfig([]byte(...))` to get a `*Config` value
- End-to-end CLI tests call `buildRootCmd()` and execute with test args
- Tests like `TestStripWrappers` and `TestCheckSafe` pass config patterns directly (already do this)

**`benchmark_test.go`:**
- Construct `*Config` once in benchmark setup (outside `b.ResetTimer()` / the hot loop)
- Pass to `ProcessWithResult` on each iteration

**`fuzz_test.go`:**
- Construct `*Config` once outside the `f.Fuzz` callback
- Capture in the closure passed to `f.Fuzz`
- Pass to `ProcessWithResult` on each fuzz iteration

**`cmd/*_test.go`:**
- Delete `resetGlobalState()` and `setupTestConfig()`
- Delete `TestIsVerbose` and `TestIsDryRun` (test functions for removed exports)
- Tests that reference package-level `rootCmd`, `validateCmd`, etc. are rewritten to use `buildRootCmd()` — each test gets its own command tree with its own config, fully isolated
- Tests call `buildRootCmd()`, set args, and execute

**`internal/testutil`:**
- `SetupTestConfig` simplified to return `*Config` directly without env var manipulation, or deleted if inline config construction suffices

**Parallel tests:**
- With no shared mutable state, `t.Parallel()` can be added to all config and hook tests

## Migration Strategy

Single-shot removal — no backward compatibility shims or deprecated functions. All callers updated in one change.

## What Stays the Same

- `internal/audit` global state (phase 2)
- `internal/logger` global state (phase 3)
- `Config` struct fields and TOML parsing logic
- Pattern compilation and matching
- `GetConfigDir()` behavior
- `EnsureConfigFiles()` and `GetDefaultConfig()` behavior
- `ProcessWithResult` still reads JSON from `io.Reader` — only config injection changes
