# Context-Aware Path Checking for MMI

## Problem

MMI currently makes approval decisions based solely on the command string. It has no awareness of _where_ a command's effects land. A command like `rm foo.txt` is treated identically whether it targets a file inside the project or in `/etc`. This means destructive commands either can't be auto-approved at all, or are approved everywhere — neither is ideal.

## Goal

Make approval decisions directory-aware for destructive commands. `rm` inside the project tree can be auto-approved; `rm` targeting `/etc` is rejected.

## Design

### Config Model

Per-pattern `paths` field on safe command entries. No global paths section.

```toml
[[commands.simple]]
name = "destructive-project-only"
commands = ["rm", "mv", "chmod", "chown"]
paths = ["$PROJECT", "/tmp"]

[[commands.simple]]
name = "read-only"
commands = ["ls", "cat", "grep"]
# No paths — works like today, no path checking

```

**Rules:**
- If a pattern has `paths`, target paths must fall within one of the listed allowed prefixes.
- If a pattern has no `paths`, no path checking is performed (today's behavior).
- `paths` is optional on `simple` pattern types. It is **not supported** on `regex` or `subcommand` patterns because there is no straightforward way to map them to command descriptors for target extraction. Config loading fails if `paths` is used on a regex or subcommand pattern.
- **Conflict resolution**: If a command appears in multiple patterns and the first match has no `paths`, but a later match does, the first match wins (no path checking). Users are responsible for not listing the same command in both path-constrained and unconstrained patterns. `mmi validate` should warn about this.

### Data Flow: paths from Config to Approval

The `paths` field flows through the system as follows:

1. **TOML parsing** (`config.go`): `parseSection()` extracts the `paths` string slice from each entry alongside existing fields.
2. **Pattern struct** (`patterns.go`): `patterns.Pattern` gains a `Paths []string` field. This carries the raw path expressions (e.g., `$PROJECT`, `/tmp`).
3. **Safe matching** (`hook.go`): `CheckSafe()` returns a `SafeResult` that includes the `Paths []string` from the matched pattern.
4. **Path checking** (`hook.go`): If `SafeResult.Paths` is non-empty, the new path checking step expands variables, extracts targets, and validates.

### Path Variables

Two variables expand at evaluation time (during step 6 of the approval flow, not at config load):

| Variable | Expansion | Source |
|----------|-----------|--------|
| `$PROJECT` | The `cwd` from the hook input | Direct from Claude Code |
| `$PROJECT_ROOT` | The main git repository root | Resolved via git, even from worktrees |

`$PROJECT_ROOT` is resolved by finding the git common directory (equivalent to `git rev-parse --git-common-dir`), which correctly traverses from worktrees to the main repo. For the common layout where worktrees live under `$PROJECT_ROOT/.claude/worktrees/`, using `$PROJECT_ROOT` in a pattern naturally covers both the main repo and all worktrees.

Literal paths (e.g., `/tmp`, `/var/tmp`) are also supported and used as-is.

**Note on variable syntax**: `$PROJECT` and `$PROJECT_ROOT` are MMI config variables, expanded at evaluation time within the `paths` field only. They are unrelated to shell variable expansion in command arguments (which is handled separately — see Ambiguous Cases). This distinction should be documented clearly in user-facing config documentation.

**Assumption**: `cwd` from the hook input is always an absolute path (Claude Code provides it this way). If it is ever relative, path resolution will produce incorrect results. The implementation should validate that `cwd` is absolute and fail closed if not.

### Command Descriptors

A registry of commands that MMI knows how to extract filesystem target paths from. Each descriptor defines how to parse the command's arguments and identify which arguments represent filesystem targets.

Lives in a new `internal/cmdpath` package.

```go
type CommandDescriptor struct {
    Name           string
    ExtractTargets func(args []string) (targets []string, unresolved []string)
}
```

The descriptor operates on the **core command after wrapper stripping** — the same string that `CheckSafe` matches against.

#### Initial Command Set

| Command | Category | Target extraction logic |
|---------|----------|----------------------|
| `rm` | Destructive | All non-flag args after `--` are targets; before `--`, args not starting with `-` are targets |
| `mv` | Destructive | All non-flag args are targets (source and dest); respects `--` |
| `chmod` | Destructive | First non-flag arg is the mode (skip it), remaining non-flag args are targets; respects `--` |
| `chown` | Destructive | First non-flag arg is the owner (skip it), remaining non-flag args are targets; respects `--` |

All descriptors handle `--` (end-of-options marker) correctly: after `--`, all remaining arguments are treated as targets regardless of whether they start with `-`.

#### Tilde Expansion

Paths starting with `~` are common in shell commands (e.g., `rm ~/foo`). Since MMI does not have access to the shell's expansion, `~` is expanded to `$HOME` from the environment before path resolution. Paths starting with `~user` are treated as unresolvable (fail closed).

#### Mode/Owner Parsing

For `chmod`, the first non-flag argument is treated as a mode if it matches the pattern `^[0-7]{3,4}$` or `^[ugoa]*[+-=][rwxXst]+$` (numeric or symbolic mode). Otherwise it is treated as a target path. This heuristic is imprecise but acceptable given the fail-closed design — worst case, a path is misidentified as a mode and the command falls through to user approval.

For `chown`, the first non-flag argument is treated as an owner if it matches `^[a-zA-Z0-9._-]+(:[a-zA-Z0-9._-]*)?$`. Same fail-closed rationale applies.

#### Config Validation

If a pattern specifies `paths` but lists a command that has no registered descriptor, config loading fails with an error. This ensures users get immediate feedback rather than silent misbehavior. `mmi validate` also surfaces this.

For `simple` patterns that list multiple commands (e.g., `commands = ["rm", "mv"]`), **every** command in the list must have a registered descriptor if `paths` is present.

### Path Resolution

1. **Extract targets** from command arguments using the command descriptor.
2. **Expand tilde**: `~` → `$HOME`, `~user` → unresolved.
3. **Resolve relative paths** against `cwd` from the hook input.
4. **Clean paths** lexically using `filepath.Clean` (no symlink following).
5. **Prefix check** each resolved target against the expanded allowed paths.

#### Ambiguous Cases

| Case | Behavior |
|------|----------|
| Glob patterns (`rm *.log`) | Resolve the base directory (`.` → cwd), check that directory is within bounds. The glob itself is not expanded. |
| Shell variable expansion (`rm $FOO`) | Cannot resolve statically → fail closed (reject, ask user). |
| Tilde (`rm ~/foo`) | Expand `~` to `$HOME`, resolve normally. |
| Tilde with user (`rm ~bob/foo`) | Cannot resolve → fail closed (reject, ask user). |
| Brace expansion (`rm {a,b}.txt`) | Treat as literal path (fail closed — won't match allowed prefix). |
| No arguments (`rm` with no args) | No targets to check → pass (the command will fail on its own). |

### Modified Approval Flow

Today's per-segment pipeline:

1. Check dangerous patterns (command substitution)
2. Check deny list
3. Strip wrappers
4. Check deny on core command
5. Check safe patterns → approve/reject

The change adds step 6 after a safe pattern matches:

1. Check dangerous patterns (command substitution)
2. Check deny list
3. Strip wrappers
4. Check deny on core command
5. Check safe patterns
6. **If matched pattern has `paths`: extract target paths → resolve against cwd → check each path is under an allowed prefix**
7. Approve/reject

**Permission decision for path violations**: `PATH_VIOLATION` produces an `ask` decision (not `deny`), consistent with the existing `NO_MATCH` behavior. The command is not inherently dangerous — it's just outside the allowed scope and needs user confirmation.

### Audit Logging

Extend the audit segment with path resolution details:

```go
type PathCheck struct {
    Targets    []string `json:"targets"`               // resolved absolute paths
    Allowed    []string `json:"allowed"`                // allowed path prefixes (expanded)
    Unresolved []string `json:"unresolved,omitempty"`   // args that couldn't be resolved
    Approved   bool     `json:"approved"`
}
```

New rejection code: `PATH_VIOLATION` — command matches a safe pattern but targets a path outside the allowed set. Added to the existing constants in `internal/audit/audit.go`.

The `PathCheck` struct is added to the existing `Segment` struct:

```go
type Segment struct {
    Command   string     `json:"command"`
    Approved  bool       `json:"approved"`
    Wrappers  []string   `json:"wrappers,omitempty"`
    Match     *Match     `json:"match,omitempty"`
    Rejection *Rejection `json:"rejection,omitempty"`
    Paths     *PathCheck `json:"paths,omitempty"`  // new
}
```

### Package Structure

New package: `internal/cmdpath`

Contains:
- `CommandDescriptor` type and the descriptor registry
- `ExtractTargets` functions for each supported command
- `ResolvePaths` function (expand variables, resolve relative, clean, prefix check)
- `ExpandPathVariables` function (expand `$PROJECT`, `$PROJECT_ROOT` in config paths)

### Testing Strategy

- **Command descriptor tests** (`internal/cmdpath/`): For each command (rm, mv, chmod, chown), test target path extraction with various flag combinations, relative/absolute paths, globs, unresolvable args, `--` handling, tilde expansion.
- **Path resolution tests** (`internal/cmdpath/`): Relative path resolution against cwd, `$PROJECT` and `$PROJECT_ROOT` expansion, `filepath.Clean` normalization (e.g., `../` traversal), absolute cwd validation.
- **Config validation tests** (`internal/config/`): `paths` on a command without a registered descriptor fails at load time. `paths` on regex or subcommand patterns fails at load time. Warning for conflicting patterns (same command with and without paths).
- **Integration tests**: End-to-end hook input with `paths` configured, verifying approve/reject based on target directory.
- **Audit tests**: `PathCheck` details appear correctly in audit log entries.

### SPEC.md Updates Required

The following sections of SPEC.md need updates after implementation:
- Section 3.2 (Configuration): Add `Paths` field to Pattern struct
- Section 4.1 (Processing Flow): Add path checking step
- Section 5.2 (Configuration Format): Document `paths` field and variables
- Section 8.7 (Rejection Codes): Add `PATH_VIOLATION`
- New section: Command Descriptor Registry

## Future Work

- **Context-shifting commands**: `cd`, `git -C`, and similar commands that change the effective working directory rather than targeting filesystem paths. These require a different descriptor model (checking where the command operates, not what files it targets) and interact with subcommand patterns in non-trivial ways.
- **Variable expansion**: Use `mvdan.cc/sh/v3/interp` to expand shell variables before extracting paths. Would allow `rm $HOME/foo.txt` to resolve correctly.
- **Additional command descriptors**: Extend the registry to cover write commands (`cp`, `touch`, `mkdir`, `tee`) and read commands if exfiltration boundaries become a concern.
- **Subcommand and regex pattern support**: Extend `paths` to work with subcommand patterns once a descriptor model for flag-based target extraction (e.g., `git -C`) is designed.
- **Symlink resolution**: Optionally resolve symlinks via `filepath.EvalSymlinks` to catch symlink-based path escapes.
