# MMI (Mother May I?)

A CLI utility that acts as a PreToolUse Hook for Claude Code, providing intelligent auto-approval of safe Bash commands.

## Overview

MMI analyzes Bash commands and automatically approves those that are known to be safe, eliminating the need for manual approval on every command. This significantly speeds up development workflows while maintaining security through a configurable allowlist approach.

The name "Mother May I?" references the childhood game where permission must be granted before taking action.

## Installation

### From Source

``` bash
just install
```

OR

```bash
go build -o mmi
mv mmi /usr/local/bin/
```

### Binary Downloads

Pre-built binaries for Linux, macOS, and Windows are available on the [Releases](https://github.com/dgerlanc/mmi/releases) page.

## Quick Start

1. Install MMI (see above)
2. Run `mmi init` to create a configuration file interactively
3. Add MMI as a hook in your Claude Code settings

## Configuration

### Claude Code Hook Setup

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

### MMI Configuration File

MMI uses a TOML configuration file at `~/.config/mmi/config.toml`. You can generate one interactively:

```bash
mmi init
```

Or set a custom location via the `MMI_CONFIG` environment variable.

### Configuration Structure

The config file has three main sections:

```toml
# Deny list - patterns always rejected (checked first)
[[deny.simple]]
name = "privilege escalation"
commands = ["sudo", "su", "doas"]

[[deny.regex]]
pattern = 'rm\s+(-[rRfF]+\s+)*/'
name = "rm root"

# Wrappers - prefixes stripped before checking core command
[[wrappers.simple]]
name = "env"
commands = ["env", "do"]

[[wrappers.command]]
command = "timeout"
flags = ["<arg>"]

# Commands - safe commands allowed to execute
[[commands.simple]]
name = "read-only"
commands = ["ls", "cat", "grep"]

[[commands.subcommand]]
command = "git"
subcommands = ["diff", "log", "status", "add"]
flags = ["-C <arg>"]

[[commands.regex]]
pattern = '^for\s+\w+\s+in\s'
name = "for loop"
```

### Config Includes

Split your configuration across multiple files:

```toml
include = ["python.toml", "rust.toml"]
```

### Config Profiles

Create named profiles in `~/.config/mmi/profiles/`:

```bash
~/.config/mmi/profiles/strict.toml
~/.config/mmi/profiles/python.toml
```

Select a profile via flag or environment variable:

```bash
mmi --profile strict
# or
MMI_PROFILE=strict mmi
```

## CLI Commands

### `mmi` (default)

Run as a hook - reads JSON from stdin, outputs approval JSON to stdout.

### `mmi init`

Interactively create a configuration file:

```bash
mmi init
mmi init --force  # Overwrite existing config
```

### `mmi validate`

Validate configuration and display compiled patterns:

```bash
mmi validate
```

### `mmi completion`

Generate shell completion scripts:

```bash
# Bash
mmi completion bash > /etc/bash_completion.d/mmi

# Zsh
mmi completion zsh > "${fpath[1]}/_mmi"

# Fish
mmi completion fish > ~/.config/fish/completions/mmi.fish

# PowerShell
mmi completion powershell > mmi.ps1
```

## Global Flags

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Enable debug logging |
| `--dry-run` | Test command approval without JSON output |
| `--profile NAME` | Use a specific config profile |
| `--no-audit-log` | Disable audit logging |

## How It Works

MMI uses a three-layer approval model:

1. **Deny List** - Patterns that are always rejected (checked first)
2. **Wrappers** - Safe command prefixes that can wrap any approved command
3. **Safe Commands** - Allowlisted commands that are safe to execute

When a command is submitted, MMI:

1. Checks for dangerous patterns (command substitution `$()` or backticks)
2. Checks if the command matches any deny patterns
3. Splits command chains (handling `&&`, `||`, `|`, `;`, `&`)
4. For each segment:
   - Checks deny list
   - Strips safe wrappers
   - Checks deny list again on core command
   - Checks if core command matches safe patterns
5. Approves only if ALL segments pass all checks

## Default Approved Commands

### Deny List (Always Rejected)

- Privilege escalation: `sudo`, `su`, `doas`
- Dangerous patterns: `rm -rf /`, `chmod 777`, `dd of=/dev/`, `mkfs.*`

### Safe Wrappers

- `timeout N` - timeout wrapper
- `nice` / `nice -n N` - process priority
- `env` - environment setup
- `VAR=value` - environment variable assignments
- Virtual environment paths (`.venv/bin/`, `venv/bin/`)
- `do` - loop body prefix

### Safe Commands

| Category | Commands |
|----------|----------|
| **Git** | `diff`, `log`, `status`, `show`, `branch`, `stash`, `bisect`, `fetch`, `add`, `checkout`, `merge`, `rebase`, `worktree` |
| **Python** | `pytest`, `python`, `ruff`, `uv`, `uvx` |
| **Node.js** | `npm` (install, run, test, build, ci), `npx` |
| **Rust** | `cargo` (build, test, run, check, clippy, fmt, clean), `maturin` |
| **Read-Only** | `ls`, `cat`, `head`, `tail`, `wc`, `find`, `grep`, `rg`, `file`, `which`, `pwd`, `du`, `df`, `curl`, `sort`, `uniq`, `cut`, `tr`, `awk`, `sed`, `xargs` |
| **File Ops** | `touch` |
| **Shell** | `echo`, `cd`, `true`, `false`, `exit`, `source`/`.` (venv activation), `sleep` |
| **Process** | `pkill`, `kill` |
| **Loops** | `for`, `while`, `done` |

## Audit Logging

MMI logs all approval decisions to `~/.local/share/mmi/audit.log` in JSON-lines format:

```json
{"timestamp":"2025-01-15T10:30:00Z","command":"git status","approved":true,"reason":"git"}
{"timestamp":"2025-01-15T10:30:05Z","command":"sudo rm -rf /","approved":false}
```

Disable with `--no-audit-log`.

## Security Model

MMI follows a **fail-secure default**:

- Deny patterns are checked first and override all approvals
- Unrecognized commands are automatically rejected
- Command substitution (`$(...)` and backticks) is always rejected
- Command chains are only approved if ALL segments are safe
- Only explicitly allowlisted patterns are allowed

## Example Configurations

The `examples/` directory contains ready-to-use configurations:

- `minimal.toml` - Bare-bones for security-conscious users
- `python.toml` - Python development
- `node.toml` - Node.js development
- `rust.toml` - Rust development
- `strict.toml` - Read-only commands only

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
go test -v ./...
```

Run with coverage:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## License

See LICENSE file for details.
