# MMI (Mother May I?)

A CLI utility that acts as a PreToolUse Hook for Claude Code, providing intelligent auto-approval of safe Bash commands.

## Overview

MMI analyzes Bash commands and automatically approves those that are known to be safe, eliminating the need for manual approval on every command. This significantly speeds up development workflows while maintaining security through a strict whitelist approach.

The name "Mother May I?" references the childhood game where permission must be granted before taking action.

## Installation

### From Source

```bash
go build -o mmi
```

### Binary

Move the compiled binary to a location in your PATH:

```bash
mv mmi /usr/local/bin/
```

## Configuration

Add MMI as a PreToolUse hook in your Claude Code settings (`~/.claude/settings.json`):

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "mmi"
          }
        ]
      }
    ]
  }
}
```

## How It Works

MMI uses a two-layer approval model:

1. **Wrappers** - Safe command prefixes that can wrap any approved command
2. **Safe Commands** - Whitelisted commands that are safe to execute

When a command is submitted, MMI:
1. Splits command chains (handling `&&`, `||`, `|`, `;`, `&`)
2. Strips safe wrappers from each segment
3. Checks if the core command is whitelisted
4. Approves only if ALL segments are safe

## Approved Commands

### Safe Wrappers

- `timeout N` - timeout wrapper
- `nice` / `nice -n N` - process priority
- `env` - environment setup
- `VAR=value` - environment variable assignments
- Virtual environment paths (`.venv/bin/`, `venv/bin/`)
- `do` - loop body prefix

### Safe Commands

**Git (Read)**
- `diff`, `log`, `status`, `show`, `branch`, `stash list`, `bisect`, `worktree list`, `fetch`

**Git (Write)**
- `add`, `checkout`, `merge`, `rebase`, `stash`

**Testing/Building**
- `pytest`, `python`, `cargo`, `make`, `maturin`, `ruff`, `uv`, `uvx`, `npm`, `npx`

**Read-Only Utilities**
- `ls`, `cat`, `head`, `tail`, `wc`, `find`, `grep`, `rg`, `file`, `which`, `pwd`, `du`, `df`, `curl`, `sort`, `uniq`, `cut`, `tr`, `awk`, `sed`, `xargs`

**File Operations**
- `touch`

**Shell Utilities**
- `echo`, `cd`, `true`, `false`, `exit`, `source`/`.` (venv activation), `sleep`

**Process Management**
- `pkill`, `kill`

**Loop Constructs**
- `for`, `while`, `done`

## Security Model

MMI follows a **fail-secure default**:

- Unrecognized commands are automatically rejected
- Command substitution (`$(...)` and backticks) is always rejected
- Command chains are only approved if ALL segments are safe
- Only explicitly whitelisted patterns are allowed

## Output Format

MMI outputs JSON decisions:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "timeout + pytest"
  }
}
```

## Testing

Run the test suite:

```bash
go test -v
```

## License

See LICENSE file for details.
