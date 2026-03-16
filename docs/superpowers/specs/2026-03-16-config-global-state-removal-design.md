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
        return nil, "", fmt.Errorf("resolving config directory: %w", err)
    }
    configPath := filepath.Join(configDir, constants.ConfigFileName)

    data, err := os.ReadFile(configPath)
    if err != nil {
        if os.IsNotExist(err) {
            cfg := loadEmbeddedDefaults()
            return cfg, "", nil
        }
        return nil, "", err
    }

    cfg, err := LoadConfigWithDir(data, configDir)
    if err != nil {
        return nil, configPath, err
    }
    return cfg, configPath, nil
}
```

Note: `GetConfigDir()` returns `(string, error)` — the error case (from `os.UserHomeDir()`) is propagated.

**Keep unchanged:**
- `LoadConfig(data []byte) (*Config, error)` — pure function, no global state
- `LoadConfigWithDir(data []byte, configDir string) (*Config, error)` — pure function
- `GetConfigDir() (string, error)` — reads env var, no mutation
- `Config` struct — unchanged

### `internal/hook` Package

**Change `ProcessWithResult` to accept `*config.Config`:**

The current signature is `ProcessWithResult(r io.Reader) Result`. It reads JSON from stdin, parses it, extracts the command, then calls `config.Get()` to get patterns.

The only change is adding `cfg *config.Config` as a parameter and removing the `config.Get()` call. The `io.Reader` parameter and all JSON parsing stay exactly as they are:

```go
func ProcessWithResult(r io.Reader, cfg *config.Config) Result
```

The caller provides config; `ProcessWithResult` no longer calls `config.Get()`.

**Change `logAudit` to accept config path and error:**

`logAudit` currently calls `config.GetConfigPath()` and `config.InitError()`. Since those functions are removed, the config path and any load error must be passed in:

```go
func logAudit(command string, approved bool, segments []audit.Segment, durationMs float64,
    sessionID, toolUseID, cwd, rawInput, rawOutput, configPath, configError string)
```

The `configPath` and `configError` values originate from `config.Load()` and flow through `ProcessWithResult` → `logAudit`. `ProcessWithResult` will need to accept these (or derive `configError` from the cfg/err it receives). The simplest approach: add `ConfigPath` and `ConfigError` fields to a new `ProcessOptions` struct, or pass them alongside `cfg`. Implementation detail to resolve during planning.

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

            if !noAuditLog {
                audit.Init("", noAuditLog)
            }
            return nil
        },
        // Default command: process stdin for command approval
        Run: func(cmd *cobra.Command, args []string) {
            runHook(cfg, cfgPath, cfgErr, dryRun)
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

**`completion.go`:** The current code references the package-level `rootCmd` in its `RunE` (for `GenBashCompletion` etc.) and uses `init()` to add itself. This becomes `buildCompletionCmd(rootCmd *cobra.Command)` which receives the root command as a parameter and returns the completion command.

**`validate.go`:** `buildValidateCmd` receives pointers to `cfg`, `cfgPath`, and `cfgErr` so it can display config status and any parse errors (replacing `config.Get()` and `config.InitError()`).

**`init.go`:** `buildInitCmd()` manages its own local flags (`force`, `configOnly`, `claudeSettings`). Does not need config since it creates it.

**`main.go`** becomes:
```go
func main() {
    if err := buildRootCmd().Execute(); err != nil {
        os.Exit(1)
    }
}
```

The `Execute()` exported function is removed — `main.go` calls `buildRootCmd().Execute()` directly.

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
result := hook.ProcessWithResult(strings.NewReader(jsonInput), cfg)
```

**`main_test.go`:**
- Remove `TestMain` that sets up global config via env vars and `config.Init()`
- Each test calls `config.LoadConfig([]byte(...))` to get a `*Config` value
- End-to-end CLI tests call `buildRootCmd()` and execute with test args

**`benchmark_test.go` and `fuzz_test.go`:**
- Both currently rely on `TestMain` for global config setup and call `hook.ProcessWithResult(io.Reader)`
- Update to construct `*Config` inline (or from a test helper) and pass to `ProcessWithResult`
- `benchmark_test.go` constructs config once in the benchmark setup, reuses across iterations

**`cmd/*_test.go`:**
- Delete `resetGlobalState()` and `setupTestConfig()`
- Tests call `buildRootCmd()`, set args, and execute
- Each test gets its own command tree with its own config — fully isolated

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
- `ProcessWithResult` still reads JSON from `io.Reader` — only config injection changes
