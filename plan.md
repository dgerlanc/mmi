# Plan: Project-Specific Configuration Files

## Overview

Add automatic discovery and merging of project-local `.mmi.toml` files. When MMI processes a command, it looks for `.mmi.toml` starting from the working directory (walking up parent dirs), then merges it with the global config. All sections (deny, commands, wrappers, subshell) are merged — project configs have the same power as the global config.

## Design

### File Discovery
- **File name:** `.mmi.toml`
- **Algorithm:** Start from `Input.Cwd` (provided by Claude Code hook input), walk up parent directories, stop at the first `.mmi.toml` found or filesystem root.
- **Control:** A `[project]` section in the global config controls behavior:
  ```toml
  [project]
  enabled = true            # default: true; set false to ignore .mmi.toml files
  search_parent_dirs = true # default: true; set false to only check cwd
  ```

### Merge Strategy
All sections from the project config are appended to the global config:

| Section | Behavior |
|---------|----------|
| `[[deny.*]]` | Appended (more deny patterns) |
| `[[commands.*]]` | Appended (more safe commands) |
| `[[wrappers.*]]` | Appended (more wrappers) |
| `[subshell]` | Project can override (last wins) |
| `include = [...]` | Honored, resolved relative to `.mmi.toml` location |
| `[project]` | Ignored in project configs (only meaningful in global) |

### Error Handling
On any error reading/parsing `.mmi.toml`, fall back to global config only (fail-secure, log debug message).

## Implementation Steps

### Step 1: Add constant
**File:** `internal/constants/constants.go`
- Add `ProjectConfigFileName = ".mmi.toml"`

### Step 2: Add `ProjectSettings` struct and parsing
**File:** `internal/config/config.go`
- Add `ProjectSettings` struct with `Enabled` and `SearchParentDirs` bool fields (both default `true`)
- Add `ProjectSettings` field to `Config` struct
- Add `ProjectConfigPath string` field to `Config` struct
- Parse `[project]` section in `loadConfigWithIncludes` after parsing `[subshell]`
- Defaults: `enabled = true`, `search_parent_dirs = true`

### Step 3: Implement `FindProjectConfig`
**File:** `internal/config/config.go`
```go
func FindProjectConfig(startDir string, searchParents bool) string
```
- Walk up from `startDir` checking for `.mmi.toml`
- If `searchParents` is false, only check `startDir`
- Return path if found, empty string otherwise

**Tests (TDD):**
- `TestFindProjectConfigInCwd` — found in current dir
- `TestFindProjectConfigInParentDir` — found in ancestor
- `TestFindProjectConfigNotFound` — returns ""
- `TestFindProjectConfigNoSearchParent` — only checks cwd
- `TestFindProjectConfigStopsAtRoot` — terminates at /

### Step 4: Implement `MergeProjectConfig`
**File:** `internal/config/config.go`
```go
func MergeProjectConfig(global *Config, project *Config) *Config
```
- Copy global config
- Append project's DenyPatterns, SafeCommands, WrapperPatterns
- Use project's SubshellAllowAll value (project overrides global)
- Preserve global's ProjectSettings (project can't change these)

**Tests (TDD):**
- `TestMergeProjectConfigAppendsDenyPatterns`
- `TestMergeProjectConfigAppendsSafeCommands`
- `TestMergeProjectConfigAppendsWrappers`
- `TestMergeProjectConfigSubshellOverride`
- `TestMergeProjectConfigPreservesProjectSettings`

### Step 5: Implement `LoadProjectConfigForCwd`
**File:** `internal/config/config.go`
```go
func LoadProjectConfigForCwd(cwd string) (*Config, string)
```
- Get global config via `Get()`
- If `ProjectSettings.Enabled` is false, return global config
- Call `FindProjectConfig(cwd, ProjectSettings.SearchParentDirs)`
- If found, read file, parse with `LoadConfigWithDir`, merge with `MergeProjectConfig`
- On any error, return global config (fail-secure)
- Return merged config and project config path

**Tests (TDD):**
- `TestLoadProjectConfigForCwd` — end-to-end with temp dir
- `TestLoadProjectConfigForCwdDisabled` — returns global when disabled
- `TestLoadProjectConfigForCwdInvalidToml` — falls back to global
- `TestLoadProjectConfigForCwdWithIncludes` — project config honors includes

### Step 6: Add `ProjectConfigPath` to audit entry
**File:** `internal/audit/audit.go`
- Add `ProjectConfigPath string \`json:"project_config_path,omitempty"\`` to `Entry` struct

### Step 7: Update `ProcessWithResult` to use project config
**File:** `internal/hook/hook.go`
- Move `cfg := config.Get()` (line 202) to after input parsing, replace with:
  ```go
  cfg, projectConfigPath := config.LoadProjectConfigForCwd(input.Cwd)
  ```
- Pass `projectConfigPath` through to `logAudit`

### Step 8: Update `logAudit` to record project config path
**File:** `internal/hook/hook.go`
- Add `projectConfigPath string` parameter to `logAudit`
- Set `ProjectConfigPath` field on `audit.Entry`
- Update all call sites (3 calls in `ProcessWithResult`)

### Step 9: Update `validate` command
**File:** `cmd/validate.go`
- After displaying global config info, check for `.mmi.toml` using `os.Getwd()` + `FindProjectConfig`
- Display project config path if found
- Parse and show project config pattern counts
- Show project settings (enabled, search_parent_dirs)

### Step 10: Update embedded default config
**File:** `internal/config/config.toml`
- Add commented `[project]` section documenting defaults

### Step 11: Update docs
**File:** `docs/SPEC.md`, `docs/PATTERNS.md`
- Document `.mmi.toml` discovery, merge behavior, and `[project]` settings
