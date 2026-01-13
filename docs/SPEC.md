# MMI (Mother May I?) - Comprehensive Specification

## 1. Overview

**MMI** is a CLI utility that acts as a **PreToolUse Hook for Claude Code**, providing intelligent auto-approval of safe Bash commands. It eliminates manual approval friction while maintaining security through configurable allowlists.

### 1.1 Problem Statement

Claude Code requires manual approval for every Bash command, which creates workflow friction during development. MMI solves this by pre-approving commands known to be safe based on configurable patterns.

### 1.2 Solution

A three-layer security model with fail-secure defaults:
1. **Deny patterns** - Always rejected (checked first)
2. **Wrappers** - Safe prefixes that can wrap commands
3. **Safe commands** - Explicitly allowlisted patterns

---

## 2. Architecture

### 2.1 Project Structure

```
mmi/
├── main.go                 # Entry point
├── cmd/                    # CLI command implementations
│   ├── root.go            # Main command, global flags
│   ├── run.go             # Hook execution (default)
│   ├── init.go            # Config initialization
│   ├── validate.go        # Config validation/display
│   └── completion.go      # Shell completions
├── internal/
│   ├── hook/              # Command approval logic
│   │   └── hook.go        # Core approval algorithm
│   ├── config/            # Configuration loading
│   │   └── config.go      # TOML parsing, includes, profiles
│   ├── patterns/          # Pattern utilities
│   │   └── patterns.go    # Regex building and compilation
│   ├── audit/             # Audit logging
│   │   └── audit.go       # JSON-lines logging
│   └── logger/            # Structured logging
│       └── logger.go      # slog-based logging
├── examples/              # Example configurations
│   ├── minimal.toml
│   ├── python.toml
│   ├── node.toml
│   ├── rust.toml
│   └── strict.toml
└── default_config.toml    # Embedded default configuration
```

### 2.2 Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/BurntSushi/toml` | TOML configuration parsing |
| `mvdan.cc/sh/v3` | Shell script parser for AST-based command splitting |
| `log/slog` | Structured logging (stdlib) |

---

## 3. Core Data Structures

### 3.1 Pattern

```go
type Pattern struct {
    Regex *regexp.Regexp  // Compiled regex for matching
    Name  string          // Human-readable description
}
```

### 3.2 Configuration

```go
type Config struct {
    WrapperPatterns []patterns.Pattern  // Layer 2: Safe prefixes
    SafeCommands    []patterns.Pattern  // Layer 3: Allowlisted commands
    DenyPatterns    []patterns.Pattern  // Layer 1: Always rejected
}
```

### 3.3 Hook Input/Output

**Input** (from Claude Code via stdin):
```go
type Input struct {
    ToolName  string            // "Bash"
    ToolInput map[string]string // {"command": "git status"}
}
```

**Output** (to Claude Code via stdout):
```go
type Output struct {
    HookSpecificOutput SpecificOutput
}

type SpecificOutput struct {
    HookEventName            string // "PreToolUse"
    PermissionDecision       string // "allow" or "ask"
    PermissionDecisionReason string // Pattern name or rejection reason
}
```

### 3.4 Audit Entry

```go
type Entry struct {
    Timestamp time.Time // UTC timestamp
    Command   string    // The command evaluated
    Approved  bool      // Approval result
    Reason    string    // Pattern name or deny reason
    Profile   string    // Configuration profile used
}
```

---

## 4. Command Approval Algorithm

### 4.1 Processing Flow

```
Claude Code Hook Input (JSON)
         │
         ▼
    ┌─────────────────────────────────┐
    │ 1. Parse & Validate Tool Type   │
    │    (Only process "Bash" tools)  │
    └─────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────┐
    │ 2. Check Dangerous Patterns     │
    │    ($() and backticks)          │
    │    Exception: Quoted heredocs   │
    └─────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────┐
    │ 3. Check Deny List              │
    │    (Absolute rejection)         │
    └─────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────┐
    │ 4. Parse & Split Command Chain  │
    │    (&&, ||, |, ;, &)            │
    │    Reject if unparseable        │
    └─────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────┐
    │ 5. For Each Segment:            │
    │    a. Check deny list           │
    │    b. Strip wrappers            │
    │    c. Check deny on core cmd    │
    │    d. Check safe patterns       │
    └─────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────┐
    │ 6. Log to Audit Trail           │
    └─────────────────────────────────┘
         │
         ▼
    Output JSON Decision (allow/ask)
```

### 4.2 Key Functions

| Function | Purpose |
|----------|---------|
| `ProcessWithResult()` | Main entry point for approval decisions |
| `containsDangerousPattern()` | Detects `$()` and backticks (excluding quoted heredocs) |
| `SplitCommandChain()` | Uses shell parser to correctly split complex commands |
| `StripWrappers()` | Removes safe prefixes to find core command |
| `CheckDeny()` | Pattern matching against deny list |
| `CheckSafe()` | Pattern matching against safe command list |

### 4.3 Dangerous Pattern Detection

Command substitution is always rejected:
- `$(...)` syntax
- Backtick syntax

**Exception**: Content inside quoted heredocs (single or double quoted delimiters) is treated as literal text:
```bash
cat > file.go << 'EOF'
fmt.Printf(`template`)  # Allowed - quoted heredoc
EOF
```

### 4.4 Command Chain Handling

Uses `mvdan.cc/sh/v3/syntax` for proper shell parsing:
- Handles: `&&`, `||`, `|`, `;`, `&`
- Extracts commands from AST nodes: `CallExpr`, `BinaryCmd`, `Subshell`, `Block`, `IfClause`, `WhileClause`, `ForClause`
- **Unparseable commands are rejected** (incomplete syntax, unclosed quotes, etc.)
- Shell loops (`while`, `for`, `if`) must be complete; their inner commands are extracted and validated individually
- **All segments must be safe** for approval

---

## 5. Configuration System

### 5.1 File Location

- Default: `~/.config/mmi/config.toml`
- Override: `MMI_CONFIG` environment variable

### 5.2 Configuration Format

```toml
# Includes (optional) - merge other configs
include = ["python.toml", "rust.toml"]

# Layer 1: Deny Patterns (checked first, override all approvals)
[[deny.simple]]
name = "privilege escalation"
commands = ["sudo", "su", "doas"]

[[deny.regex]]
pattern = 'rm\s+(-[rRfF]+\s+)*/'
name = "rm root"

# Layer 2: Wrappers (safe prefixes stripped before checking)
[[wrappers.command]]
command = "timeout"
flags = ["<arg>"]

[[wrappers.regex]]
pattern = '^([A-Z_][A-Z0-9_]*=[^\s]*\s+)+'
name = "env vars"

# Layer 3: Safe Commands (allowlisted patterns)
[[commands.simple]]
name = "read-only"
commands = ["ls", "cat", "grep", "head", "tail"]

[[commands.subcommand]]
command = "git"
subcommands = ["diff", "log", "status", "branch"]
flags = ["-C <arg>"]

[[commands.regex]]
pattern = '^(true|false|exit(\s+\d+)?)$'
name = "shell builtin"
```

### 5.3 Pattern Types

| Type | Description | Example |
|------|-------------|---------|
| `simple` | Exact command match (any args) | `["ls", "cat", "grep"]` |
| `subcommand` | Command + specific subcommands | `git` with `["diff", "log"]` |
| `command` | Command with flag patterns | `timeout` with `["<arg>"]` |
| `regex` | Custom regex pattern | `^pytest\b` |

### 5.4 Pattern Building

| Input | Output Regex |
|-------|--------------|
| `BuildSimplePattern("pytest")` | `^pytest\b` |
| `BuildSubcommandPattern("git", ["status", "log"], [])` | `^git\s+(status\|log)\b` |
| `BuildWrapperPattern("timeout", ["<arg>"])` | `^timeout\s+(\S+\s+)?` |

### 5.5 Profiles

Alternative configurations stored in `~/.config/mmi/profiles/<name>.toml`:
- Selected via `--profile NAME` flag or `MMI_PROFILE` env var
- Completely replaces default config (not merged)

### 5.6 Embedded Default Config

- Restrictive by design (fail-secure)
- Used when no config file exists
- Embedded via `go:embed`

---

## 6. CLI Interface

### 6.1 Commands

| Command | Description |
|---------|-------------|
| `mmi` (default) | Process hook input from stdin, output decision to stdout |
| `mmi init` | Create default config file |
| `mmi validate` | Display compiled patterns from config |
| `mmi completion` | Generate shell completions (bash, zsh, fish, powershell) |

### 6.2 Global Flags

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Enable debug logging |
| `--dry-run` | Test commands without JSON output |
| `--profile NAME` | Use specific config profile |
| `--no-audit-log` | Disable audit logging |

### 6.3 Init Command Flags

| Flag | Description |
|------|-------------|
| `-f, --force` | Overwrite existing config file |

---

## 7. Claude Code Integration

### 7.1 Hook Configuration

Add to `~/.claude/settings.json`:
```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{
        "type": "command",
        "command": "mmi"
      }]
    }]
  }
}
```

### 7.2 Hook Protocol

**Input** (stdin JSON from Claude Code):
```json
{
  "tool_name": "Bash",
  "tool_input": {
    "command": "git status"
  }
}
```

**Output** (stdout JSON to Claude Code):
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "git"
  }
}
```

**Permission Decisions**:
- `allow` - Command auto-approved
- `ask` - User confirmation required

---

## 8. Audit Logging

### 8.1 Location

`~/.local/share/mmi/audit.log`

### 8.2 Format

JSON-lines (one JSON object per line):
```json
{"timestamp":"2025-01-15T10:30:00Z","command":"git status","approved":true,"reason":"git","profile":""}
{"timestamp":"2025-01-15T10:30:05Z","command":"sudo rm -rf /","approved":false,"reason":"privilege escalation"}
```

### 8.3 Fields

| Field | Description |
|-------|-------------|
| `timestamp` | UTC timestamp |
| `command` | The command evaluated |
| `approved` | Boolean approval result |
| `reason` | Pattern name (approved) or deny reason (rejected) |
| `profile` | Configuration profile used (empty = default) |

---

## 9. Security Model

### 9.1 Fail-Secure Defaults

- Deny patterns checked **first** and override all approvals
- Unrecognized commands automatically **rejected**
- Unparseable commands (incomplete syntax, unclosed quotes) **rejected**
- Command substitution always **rejected** (except in quoted heredocs)
- Command chains only approved if **ALL segments** are safe
- Only **explicitly allowlisted** patterns are approved

### 9.2 Safety Features

- Shell parser (`mvdan.cc/sh`) for correct AST-based command splitting
- Quoted heredoc detection prevents false positives on literal backticks
- Segment-by-segment chain validation
- Wrapper stripping before core command validation
- Re-check deny patterns after wrapper stripping

### 9.3 No Bypass Paths

- Silent failures for unparseable JSON input
- Explicit rejection for unparseable shell commands
- Graceful fallback to embedded defaults if config missing
- Profile specified but not found = error (not fallback)

---

## 10. Testing

### 10.1 Test Structure

| Test File | Coverage |
|-----------|----------|
| `main_test.go` | Integration tests, config loading |
| `benchmark_test.go` | Performance benchmarks |
| `fuzz_test.go` | Fuzzing tests for robustness |
| `internal/config/config_test.go` | Configuration validation |
| `internal/hook/hook_test.go` | Approval logic, dangerous patterns |
| `internal/audit/audit_test.go` | Audit logging |
| `internal/logger/logger_test.go` | Logging |
| `internal/patterns/patterns_test.go` | Pattern compilation |
| `cmd/*_test.go` | CLI command tests |

### 10.2 Test Commands

```bash
just test              # Run tests (excludes fuzz)
just coverage          # Run with coverage summary
just coverage-html     # Generate HTML coverage report
just bench             # Run benchmarks
just fuzz time=30s     # Run fuzz tests
```

### 10.3 Fuzz Targets

| Target | Purpose |
|--------|---------|
| `FuzzSplitCommandChain` | Parsing arbitrary commands |
| `FuzzProcess` | Processing arbitrary JSON |
| `FuzzStripWrappers` | Wrapper stripping edge cases |
| `FuzzCheckSafe` | Safe checking with random input |
| `FuzzCheckDeny` | Deny checking with random input |

---

## 11. Build and Release

### 11.1 Build Commands

```bash
just build        # Build binary: go build -o mmi
just install      # Install to /usr/local/bin
just check        # Run fmt-check, vet, and test
just ci           # Full CI simulation
```

### 11.2 Release Process

**GoReleaser** (`.goreleaser.yaml`):
- Cross-compile: Linux, Darwin, Windows (amd64, arm64)
- Stripped binaries with version info via ldflags
- CGO disabled for static builds
- GitHub Releases with auto-generated changelog
- Homebrew tap: `dgerlanc/homebrew-tap`

### 11.3 CI/CD

**GitHub Actions Workflows**:
- `ci.yml` - Push/PR: format check, tests, build verification
- `release.yml` - Tags: GoReleaser publish

---

## 12. Environment Variables

| Variable | Purpose |
|----------|---------|
| `MMI_CONFIG` | Custom config directory |
| `MMI_PROFILE` | Default profile name |

---

## 13. Example Configurations

| File | Purpose |
|------|---------|
| `minimal.toml` | Read-only commands only |
| `python.toml` | Python dev (pytest, uv, ruff, mypy, pip) |
| `node.toml` | Node.js dev (npm, yarn, pnpm, eslint, tsc) |
| `rust.toml` | Rust dev (cargo, rustup, maturin) |
| `strict.toml` | Ultra-restrictive, read-only only |

---

## 14. Version Information

Build-time variables injected via ldflags:
- `main.version` - Version tag
- `main.commit` - Git commit hash
- `main.date` - Build date
