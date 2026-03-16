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
    configDir := GetConfigDir()
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

**Keep unchanged:**
- `LoadConfig(data []byte) (*Config, error)` — pure function, no global state
- `LoadConfigWithDir(data []byte, configDir string) (*Config, error)` — pure function
- `GetConfigDir() string` — reads env var, no mutation
- `Config` struct — unchanged

### `internal/hook` Package

**Change signatures:**
```go
func Process(command string, cfg *config.Config) int
func ProcessWithResult(command string, cfg *config.Config) Result
```

Remove the `config.Get()` call inside `ProcessWithResult`. The caller provides config.

`StripWrappers` and `CheckSafe` already accept pattern slices as parameters — no changes.

### `cmd` Package

**Replace `init()` + global flags with a builder function:**

```go
func buildRootCmd() *cobra.Command {
    var (
        verbose    bool
        dryRun     bool
        noAuditLog bool
        cfg        *config.Config
        cfgPath    string
    )

    rootCmd := &cobra.Command{
        Use:   "mmi",
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            logger.Init(verbose)

            var err error
            cfg, cfgPath, err = config.Load()
            if err != nil {
                // handle error
            }

            if !noAuditLog {
                audit.Init(cfgPath, false)
            }
            return nil
        },
    }

    rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
    rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "dry run mode")
    rootCmd.PersistentFlags().BoolVar(&noAuditLog, "no-audit-log", false, "disable audit logging")

    rootCmd.AddCommand(buildRunCmd(&cfg, &dryRun))
    rootCmd.AddCommand(buildValidateCmd(&cfg, &cfgPath))
    rootCmd.AddCommand(buildInitCmd())

    return rootCmd
}
```

Config flows from `PersistentPreRunE` to subcommands via pointer variables in closure scope. This works because `PersistentPreRunE` always executes before `RunE`.

**Subcommand builders** (e.g., `buildRunCmd`, `buildValidateCmd`) accept pointers to the config and flags they need, closing over them in their `RunE` functions.

**`buildInitCmd()`** manages its own local flags (`force`, `configOnly`, `claudeSettings`) — it does not need config since it creates it.

**`main.go`** becomes:
```go
func main() {
    if err := buildRootCmd().Execute(); err != nil {
        os.Exit(1)
    }
}
```

### Test Changes

**`internal/config` tests:**
- Remove all `Reset()` and `Init()` calls
- Tests use `LoadConfig()` / `LoadConfigWithDir()` directly — these are pure functions that return values
- No global state to manage

**`internal/hook` tests:**
- Construct `*config.Config` literals inline and pass to `ProcessWithResult`:
```go
cfg := &config.Config{
    SafeCommands:    []patterns.Pattern{...},
    WrapperPatterns: []patterns.Pattern{...},
}
result := hook.ProcessWithResult("git status", cfg)
```

**`main_test.go`:**
- Remove `TestMain` that sets up global config via env vars and `config.Init()`
- Each test calls `config.LoadConfig([]byte(...))` to get a `*Config` value
- End-to-end CLI tests call `buildRootCmd()` and execute with test args

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
