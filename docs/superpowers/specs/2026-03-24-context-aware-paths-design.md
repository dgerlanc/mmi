# Context-Aware Path Checking for MMI

## Problem

MMI currently makes approval decisions based solely on the command string. It has no awareness of _where_ a command's effects land. A command like `rm foo.txt` is treated identically whether it targets a file inside the project or in `/etc`. This means destructive commands either can't be auto-approved at all, or are approved everywhere — neither is ideal.

## Goal

Make approval decisions directory-aware for destructive and context-shifting commands. `rm` inside the project tree can be auto-approved; `rm` targeting `/etc` is rejected.

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

[[commands.subcommand]]
command = "git"
subcommands = ["diff", "log", "status"]
flags = ["-C <arg>"]
paths = ["$PROJECT_ROOT"]
```

**Rules:**
- If a pattern has `paths`, target paths must fall within one of the listed allowed prefixes.
- If a pattern has no `paths`, no path checking is performed (today's behavior).
- `paths` is optional on any pattern type (simple, subcommand, command, regex).

### Path Variables

Two variables expand at evaluation time:

| Variable | Expansion | Source |
|----------|-----------|--------|
| `$PROJECT` | The `cwd` from the hook input | Direct from Claude Code |
| `$PROJECT_ROOT` | The main git repository root | Resolved via git, even from worktrees |

`$PROJECT_ROOT` is resolved by finding the git common directory (equivalent to `git rev-parse --git-common-dir`), which correctly traverses from worktrees to the main repo. For the common layout where worktrees live under `$PROJECT_ROOT/.claude/worktrees/`, using `$PROJECT_ROOT` in a pattern naturally covers both the main repo and all worktrees.

Literal paths (e.g., `/tmp`, `/var/tmp`) are also supported and used as-is.

### Command Descriptors

A registry of commands that MMI knows how to extract filesystem target paths from. Each descriptor defines how to parse the command's arguments and identify which arguments represent filesystem targets.

```go
type CommandDescriptor struct {
    Name       string
    ExtractTargets func(args []string) (targets []string, unresolved []string)
}
```

#### Initial Command Set

| Command | Category | Target extraction logic |
|---------|----------|----------------------|
| `rm` | Destructive | All non-flag args are targets |
| `mv` | Destructive | All args are targets (source and dest) |
| `chmod` | Destructive | All non-flag, non-mode args are targets |
| `chown` | Destructive | All non-flag, non-owner args are targets |
| `cd` | Context-shifting | First non-flag arg is target |
| `git -C` | Context-shifting | The arg after `-C` is the effective directory |

#### Config Validation

If a pattern specifies `paths` but lists a command that has no registered descriptor, config loading fails with an error. This ensures users get immediate feedback rather than silent misbehavior. `mmi validate` also surfaces this.

### Path Resolution

1. **Extract targets** from command arguments using the command descriptor.
2. **Resolve relative paths** against `cwd` from the hook input.
3. **Clean paths** lexically using `filepath.Clean` (no symlink following).
4. **Prefix check** each resolved target against the expanded allowed paths.

#### Ambiguous Cases

| Case | Behavior |
|------|----------|
| Glob patterns (`rm *.log`) | Resolve the base directory (`.` → cwd), check that directory is within bounds. The glob itself is not expanded. |
| Variable expansion (`rm $FOO`) | Cannot resolve statically → fail closed (reject, ask user). |
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

New rejection code: `PATH_VIOLATION` — command matches a safe pattern but targets a path outside the allowed set.

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

### Testing Strategy

- **Command descriptor tests**: For each command (rm, mv, chmod, chown, cd, git -C), test target path extraction with various flag combinations, relative/absolute paths, globs, unresolvable args.
- **Path resolution tests**: Relative path resolution against cwd, `$PROJECT` and `$PROJECT_ROOT` expansion, `filepath.Clean` normalization (e.g., `../` traversal).
- **Integration tests**: End-to-end hook input with `paths` configured, verifying approve/reject based on target directory.
- **Config validation tests**: `paths` on a command without a registered descriptor fails at load time.
- **Audit tests**: `PathCheck` details appear correctly in audit log entries.

## Future Work

- **Variable expansion**: Use `mvdan.cc/sh/v3/interp` to expand shell variables before extracting paths. Would allow `rm $HOME/foo.txt` to resolve correctly.
- **Additional command descriptors**: Extend the registry to cover write commands (`cp`, `touch`, `mkdir`, `tee`) and read commands if exfiltration boundaries become a concern.
- **Symlink resolution**: Optionally resolve symlinks via `filepath.EvalSymlinks` to catch symlink-based path escapes.
