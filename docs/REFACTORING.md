# Refactoring Plan

This document outlines refactoring opportunities for the MMI codebase. These are structural improvements only - no new features.

## Summary

The codebase is well-structured overall with good separation into internal packages and comprehensive test coverage. However, there are opportunities to improve maintainability, testability, and code clarity.

**Priority Legend:**
- P1: Low risk, high value - do first
- P2: Medium risk, medium value
- P3: Higher risk or lower value - evaluate carefully

---

## P1: Remove Dead Code [COMPLETED]

### 1.1 Remove Duplicate Check Functions [COMPLETED]

**Files:** `internal/hook/hook.go`

**Issue:** Two pairs of functions do the same thing but return different types:

```go
// Lines 302-309: Returns simple string
func CheckDeny(cmd string, denyPatterns []patterns.Pattern) string

// Lines 342-353: Returns detailed struct
func CheckDenyWithResult(cmd string, denyPatterns []patterns.Pattern) DenyResult

// Lines 558-565: Returns simple string
func CheckSafe(cmd string, safeCommands []patterns.Pattern) string

// Lines 320-332: Returns detailed struct
func CheckSafeWithResult(cmd string, safeCommands []patterns.Pattern) SafeResult
```

**Analysis:**
- Production code only uses `CheckDenyWithResult` and `CheckSafeWithResult`
- `CheckDeny` and `CheckSafe` are only called from tests (`main_test.go:299`, `main_test.go:323`)

**Action:**
1. Update tests to use `WithResult` variants
2. Remove `CheckDeny()` (lines 300-309)
3. Remove `CheckSafe()` (lines 556-565)
4. Consider renaming `CheckDenyWithResult` → `CheckDeny` and `CheckSafeWithResult` → `CheckSafe`

**Benefit:** ~30 lines removed, cleaner API

---

## P1: Fix Silent Error Handling

### 1.2 Handle JSON Marshal Errors

**Files:** `internal/hook/hook.go`

**Issue:** JSON marshaling errors are silently ignored:

```go
// Line 380
data, _ := json.Marshal(output)
return string(data)

// Line 393
data, _ := json.Marshal(output)
return string(data)
```

**Risk:** While `json.Marshal` rarely fails for these simple structs, silent failures make debugging harder.

**Action:**
```go
func FormatApproval(reason string) string {
    output := Output{...}
    data, err := json.Marshal(output)
    if err != nil {
        // Log error and return a safe default
        logger.Debug("failed to marshal approval output", "error", err)
        return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"internal error"}}`
    }
    return string(data)
}
```

**Benefit:** Better debugging, fail-safe behavior

---

## P1: Extract Shared Test Utilities

### 1.3 Create Test Helper Package

**Files:** Multiple `*_test.go` files

**Issue:** Test setup code is duplicated across test files:

| File | Setup Function |
|------|----------------|
| `cmd/run_test.go:16` | `setupTestConfig()` |
| `cmd/root_test.go:11` | `resetGlobalState()` |
| `main_test.go:98` | `TestMain()` setup |
| `cmd/init_test.go` | Similar patterns |

**Action:**
Create `internal/testutil/testutil.go`:

```go
package testutil

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/dgerlanc/mmi/internal/config"
)

// SetupTestConfig creates a temporary config directory with test configuration.
// Returns a cleanup function that should be deferred.
func SetupTestConfig(t *testing.T, configContent string) func() {
    t.Helper()

    tmpDir := t.TempDir()
    os.Setenv("MMI_CONFIG", tmpDir)

    if configContent != "" {
        configPath := filepath.Join(tmpDir, "config.toml")
        if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
            t.Fatal(err)
        }
    }

    config.Reset()
    config.Init()

    return func() {
        os.Unsetenv("MMI_CONFIG")
        config.Reset()
    }
}

// MinimalTestConfig returns a minimal config for testing.
const MinimalTestConfig = `
[[commands.simple]]
name = "safe"
commands = ["ls", "cat", "echo"]

[[deny.simple]]
name = "dangerous"
commands = ["rm"]
`
```

**Benefit:** DRY principle, consistent test setup, easier maintenance

---

## P2: Break Up hook.go

### 2.1 Extract Command Parsing

**Files:** `internal/hook/hook.go` (565 lines)

**Issue:** `hook.go` handles too many responsibilities:
1. JSON parsing and I/O types (lines 19-56)
2. Dangerous pattern detection with heredoc handling (lines 58-148)
3. Main processing logic (lines 150-298)
4. Pattern matching (lines 300-353)
5. Audit logging helper (lines 355-369)
6. JSON formatting (lines 371-395)
7. Command chain splitting (lines 397-534)
8. Wrapper stripping (lines 536-554)

**Action:** Split into focused files:

```
internal/hook/
├── hook.go           # ProcessWithResult, Process, types (Input, Output, Result)
├── parser.go         # SplitCommandChain, extractCommands, ErrUnparseable
├── dangerous.go      # containsDangerousPattern, findQuotedHeredocRanges, dangerousPattern
├── matcher.go        # CheckDeny*, CheckSafe*, StripWrappers, SafeResult, DenyResult
└── format.go         # FormatApproval, FormatAsk
```

**Migration Strategy:**
1. Create new files with functions moved (no logic changes)
2. Update imports as needed (all stay in `hook` package)
3. Run tests to verify no regressions

**Benefit:**
- Each file ~100-150 lines (easier to understand)
- Clear separation of concerns
- Easier to test individual components
- Simplifies future modifications

---

## P2: Reduce Code Duplication in Config Parsing

### 2.2 Consolidate parseSection and parseDenySection

**Files:** `internal/config/config.go`

**Issue:** `parseSection()` (lines 68-162) and `parseDenySection()` (lines 291-343) share significant logic:
- Both handle `simple` and `regex` subsections identically
- Only difference: `parseSection` also handles `command` and `subcommand`

**Current:**
```go
func parseSection(sectionData map[string]any, isWrapper bool, sectionName string) ([]patterns.Pattern, error)
func parseDenySection(sectionData map[string]any) ([]patterns.Pattern, error)
```

**Action:** Refactor to use a single parser with options:

```go
type sectionOptions struct {
    name           string  // For error messages: "wrappers", "commands", "deny"
    isWrapper      bool    // Affects pattern generation for simple commands
    allowSubcommand bool   // Whether to parse subcommand entries
    allowCommand   bool    // Whether to parse command entries
}

func parseSection(sectionData map[string]any, opts sectionOptions) ([]patterns.Pattern, error) {
    // Unified implementation
}
```

Then:
```go
// For wrappers
parseSection(data, sectionOptions{name: "wrappers", isWrapper: true, allowCommand: true})

// For commands
parseSection(data, sectionOptions{name: "commands", allowSubcommand: true})

// For deny
parseSection(data, sectionOptions{name: "deny"})
```

**Benefit:** ~50 lines removed, single source of truth for parsing logic

---

## P2: Improve Type Organization

### 2.3 Consolidate Type Definitions

**Issue:** Related types are scattered across packages:

| Package | Types |
|---------|-------|
| `hook` | Input, Output, SpecificOutput, Result, ToolInputData, SafeResult, DenyResult |
| `config` | Config |
| `patterns` | Pattern |
| `audit` | Entry, Segment, Match, Rejection |

**Problem:** Understanding data flow requires reading 4 different files.

**Options:**

**Option A: Document current structure** (minimal change)
- Add package-level doc comments explaining type relationships
- Add cross-references in godoc comments

**Option B: Create types.go in hook package** (moderate change)
- Move hook-specific types to `internal/hook/types.go`
- Keep other packages' types where they are

**Option C: Create dedicated types package** (larger change)
- Create `internal/types/types.go` with all shared types
- Other packages import from types

**Recommendation:** Start with Option A, consider Option B if hook.go is being split anyway.

---

## P3: Consolidate Global State

### 3.1 Create Application Context

**Files:** Multiple packages

**Issue:** Global state is scattered:

```go
// cmd/root.go:11-16
var (
    verbose    bool
    dryRun     bool
    noAuditLog bool
)

// config/config.go:29-34
var (
    globalConfig      *Config
    configInitialized bool
)

// audit/audit.go (similar pattern)
// logger/logger.go (similar pattern)
```

**Problems:**
- Tests must carefully reset state between runs
- Initialization order is implicit
- Hard to run multiple configurations in parallel (e.g., for testing)

**Action:** Create application context:

```go
// internal/app/app.go
package app

type App struct {
    Config *config.Config
    Logger *slog.Logger
    Audit  *audit.Logger

    Verbose    bool
    DryRun     bool
    NoAuditLog bool
}

type Option func(*App)

func New(opts ...Option) (*App, error) {
    app := &App{}
    for _, opt := range opts {
        opt(app)
    }
    // Initialize components
    return app, nil
}

func WithVerbose(v bool) Option {
    return func(a *App) { a.Verbose = v }
}
```

**Migration:**
1. Create `App` struct
2. Update commands to accept `*App` parameter
3. Update hook.ProcessWithResult to accept config parameter
4. Gradually remove global state

**Risk:** Moderate refactoring effort, touching many files

**Benefit:**
- Testability improves significantly
- Clear initialization order
- Can run multiple instances (useful for parallel tests)

---

## P3: Performance Optimizations (Future)

### 3.2 Pattern Matching Optimization

**Files:** `internal/hook/hook.go` lines 234-263

**Current:** O(n) sequential scan through all patterns for each command segment.

**Status:** Not needed now. Current performance is fine for typical configs (~100 patterns).

**If needed later:**
- Index patterns by first word of command
- Use a trie for command prefix matching
- Pre-sort patterns by frequency of match

---

## Implementation Order

### Phase 1: Quick Wins (P1 items)
1. Remove duplicate `CheckDeny`/`CheckSafe` functions
2. Fix silent JSON error handling
3. Create shared test utilities

### Phase 2: Code Organization (P2 items)
1. Split `hook.go` into focused files
2. Consolidate config parsing functions
3. Improve type documentation

### Phase 3: Architecture (P3 items)
1. Evaluate need for application context
2. Implement if testing pain points justify effort

---

## Verification Checklist

For each refactoring:
- [ ] All existing tests pass
- [ ] No new test file imports (except testutil)
- [ ] `go vet` reports no issues
- [ ] `go build` succeeds
- [ ] Coverage does not decrease
- [ ] Benchmark performance unchanged

---

## Notes

- This plan focuses on structural improvements only
- No new features or behavior changes
- Each item is independent and can be done separately
- Start with P1 items to build confidence
