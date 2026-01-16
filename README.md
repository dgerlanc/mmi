<img src="logo.png" alt="MMI Logo" height="250">

# mmi (Mother May I?)

A CLI utility that acts as a PreToolUse Hook for Claude Code, providing intelligent auto-approval of safe Bash commands.

## Overview

MMI parses Bash commands and automatically approves those that are known to be safe, eliminating the need for manual approval on every command. This significantly speeds up development workflows while maintaining security through a configurable allowlist approach.

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

1. Install `mmi` (see above)
2. Run `mmi init` to create a default configuration file
3. Add `mmi` as a hook in your Claude Code settings
4. (Optional) Include an example config for your language stack (see [Example Configurations](#example-configurations))

## Configuration

### Claude Code Hook Setup

Add `mmi` as a PreToolUse hook in your Claude Code settings (`~/.claude/settings.json`):

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

### `mmi` Configuration File

`mmi` uses a TOML configuration file at `~/.config/mmi/config.toml`. Generate the default config:

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
pattern = '^(true|false|exit(\s+\d+)?)$'
name = "shell builtin"
```

### Config Includes

Split your configuration across multiple files:

```toml
include = ["python.toml", "rust.toml"]
```

To use different configurations for different projects, set the `MMI_CONFIG` environment variable to point to a different config directory.

## CLI Commands

### `mmi` (default)

Run as a hook - reads JSON from stdin, outputs approval JSON to stdout.

### `mmi init`

Create a default configuration file:

```bash
mmi init
mmi init --force  # Overwrite existing config
```

The default config includes basic Unix utilities and shell builtins. For language-specific commands (Python, Node.js, Rust), copy an example config from `examples/`.

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
| `--no-audit-log` | Disable audit logging |

## How It Works

`mmi` uses a three-layer approval model:

1. **Deny List** - Patterns that are always rejected (checked first)
2. **Wrappers** - Safe command prefixes that can wrap any approved command
3. **Safe Commands** - Allowlisted commands that are safe to execute

When a command is submitted, `mmi`:

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

The default configuration is intentionally restrictive. Use example configs for language-specific setups.

### Deny List (Always Rejected)

- Privilege escalation: `sudo`, `su`, `doas`
- Dangerous patterns: `rm -rf /`, `chmod 777`, `dd of=/dev/`, `mkfs.*`

### Safe Wrappers

- `timeout N` - timeout wrapper
- `nice` / `nice -n N` - process priority
- `env` - environment setup
- `VAR=value` - environment variable assignments
- `do` - loop body prefix

### Safe Commands (Default Config)

| Category | Commands |
|----------|----------|
| **Unix Utilities** | `ls`, `cat`, `head`, `tail`, `wc`, `find`, `grep`, `rg`, `file`, `which`, `pwd`, `du`, `df`, `curl`, `sort`, `uniq`, `cut`, `tr`, `awk`, `sed`, `xargs` |
| **File Ops** | `touch`, `make` |
| **Shell** | `echo`, `cd`, `true`, `false`, `exit`, `sleep` |

### Additional Commands (via Example Configs)

Copy from `examples/` to enable language-specific commands:

| Config | Enables |
|--------|---------|
| `python.toml` | `pytest`, `python`, `ruff`, `uv`, `uvx`, `mypy`, `black`, `isort`, `pip`, git subcommands |
| `node.toml` | `npm`, `npx`, `node`, `yarn`, `pnpm`, `bun`, `eslint`, `prettier`, `tsc`, git subcommands |
| `rust.toml` | `cargo`, `rustup`, `maturin`, `rustc`, `rustfmt`, git subcommands |
| `minimal.toml` | Basic read-only commands plus git read-only (`status`, `log`, `diff`, `show`, `branch`) |
| `strict.toml` | Read-only only, denies file modifications |

## Audit Logging

`mmi` logs all approval decisions to `~/.local/share/mmi/audit.log` in JSON-lines format:

```json
{"timestamp":"2025-01-15T10:30:00Z","command":"git status","approved":true,"reason":"git"}
{"timestamp":"2025-01-15T10:30:05Z","command":"sudo rm -rf /","approved":false}
```

Disable with `--no-audit-log`.

## Security Model

`mmi` follows a **fail-secure default**:

- Deny patterns are checked first and override all approvals
- Unrecognized commands are automatically rejected
- Unparseable commands (incomplete syntax, unclosed quotes) are rejected
- Command substitution (`$(...)` and backticks) is always rejected
- Command chains are only approved if ALL segments are safe
- Only explicitly allowlisted patterns are allowed
- Shell loops (`while`, `for`) must be complete; their inner commands are extracted and validated individually

## Example Configurations

The `examples/` directory contains ready-to-use configurations:

- `minimal.toml` - Bare-bones for security-conscious users
- `python.toml` - Python development (pytest, uv, ruff, mypy, etc.)
- `node.toml` - Node.js development (npm, yarn, pnpm, bun, etc.)
- `rust.toml` - Rust development (cargo, rustup, maturin, etc.)
- `strict.toml` - Read-only commands only

To use an example config:

```bash
# Replace default config with an example
cp examples/python.toml ~/.config/mmi/config.toml

# Or use includes to combine configs
echo 'include = ["python.toml"]' >> ~/.config/mmi/config.toml
cp examples/python.toml ~/.config/mmi/
```

## Output Format

`mmi` outputs JSON decisions:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "timeout + pytest"
  }
}
```

## FAQ

### What happens if I don't run `mmi init` first?

If no configuration file exists at `~/.config/mmi/config.toml` (or the path specified by `MMI_CONFIG`), `mmi` will reject all commands. This fail-secure behavior ensures that commands are never auto-approved without explicit configuration. Run `mmi init` to create a default configuration file.

### Why are command substitutions (`$(...)` and backticks) always rejected?

Command substitution can execute arbitrary commands inside what appears to be a safe command. For example, `echo $(rm -rf /)` looks like an echo command but actually deletes files. `mmi` rejects both `$(...)` and backtick syntaxes unconditionally for security.

### How do I test if a command will be approved?

Use `mmi validate` to see your compiled patterns, or use the `--dry-run` flag to test specific commands without producing JSON output. Add `--verbose` for detailed debug logs showing why a command was approved or rejected.

### Can I have different configurations for different projects?

Yes, use the `MMI_CONFIG` environment variable to point to a different config directory. For example, set `MMI_CONFIG=/path/to/project/.mmi` to use a project-specific configuration.

### How do wrappers work?

Wrappers are safe prefixes that are stripped before checking the core command. For example, if `timeout` is a wrapper and `pytest` is approved, then `timeout 10 pytest` is approved. Wrappers don't make unsafe commands safe—they simply allow safe commands to be wrapped with approved prefixes.

### Where are approval decisions logged?

Audit logs are written to `~/.local/share/mmi/audit.log` in JSON-lines format. Each entry includes the timestamp, command, approval status, and the pattern name that matched (if approved). Disable logging with `--no-audit-log`.

### Why is my command rejected even though I added it to my config?

Common causes:
- **Deny list priority**: Deny patterns are checked first and override all approvals
- **Command substitution**: Commands containing `$(...)` or backticks are always rejected
- **Command chains**: If using `&&`, `||`, `|`, or `;`, all segments must be approved
- **Pattern mismatch**: Use `mmi validate` to verify your patterns and `--verbose` to see why rejection occurred

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

## Creating Releases

Releases are automated via GitHub Actions and GoReleaser.

### Steps

1. **Update the changelog** - Move items from `[Unreleased]` to a versioned section in `CHANGELOG.md`:
   ```markdown
   ## [0.1.0] - 2026-01-13
   ```

2. **Create and push a git tag**:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

3. **Automated release** - GitHub Actions will automatically:
   - Build binaries for Linux, macOS, Windows (amd64 + arm64)
   - Create the GitHub release with archives and checksums
   - Update the Homebrew tap (`dgerlanc/homebrew-tap`)

### Test Locally First (Optional)

```bash
just release-test
```

This validates the GoReleaser config and performs a dry-run snapshot build.

## License

See LICENSE file for details.
