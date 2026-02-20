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
│   │   └── config.go      # TOML parsing, includes
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
// ToolInputData represents the tool_input field
type ToolInputData struct {
    Command     string // Required: the bash command
    Description string // Optional: command description
    Timeout     int    // Optional: timeout in milliseconds
}

type Input struct {
    SessionID      string        // Required: session identifier
    TranscriptPath string        // Required: path to transcript file
    Cwd            string        // Required: current working directory
    PermissionMode string        // Required: permission mode
    HookEventName  string        // Required: "PreToolUse"
    ToolName       string        // Required: "Bash"
    ToolInput      ToolInputData // Required: tool-specific input
    ToolUseID      string        // Required: tool use identifier
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

### 3.4 Audit Entry (v1)

```go
type Entry struct {
    Version     int       `json:"version"`
    ToolUseID   string    `json:"tool_use_id"`
    SessionID   string    `json:"session_id"`
    Timestamp   time.Time `json:"timestamp"`
    DurationMs  float64   `json:"duration_ms"`
    Command     string    `json:"command"`
    Approved    bool      `json:"approved"`
    Segments    []Segment `json:"segments"`
    Cwd         string    `json:"cwd"`
    Input       string    `json:"input"`
    Output      string    `json:"output"`
    ConfigPath  string    `json:"config_path"`
    ConfigError string    `json:"config_error,omitempty"`
}

type Segment struct {
    Command   string     `json:"command"`
    Approved  bool       `json:"approved"`
    Wrappers  []string   `json:"wrappers,omitempty"`
    Match     *Match     `json:"match,omitempty"`
    Rejection *Rejection `json:"rejection,omitempty"`
}

type Match struct {
    Type    string `json:"type"`
    Pattern string `json:"pattern,omitempty"`
    Name    string `json:"name"`
}

type Rejection struct {
    Code    string `json:"code"`
    Name    string `json:"name,omitempty"`
    Pattern string `json:"pattern,omitempty"`
    Detail  string `json:"detail,omitempty"`
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
    │ 2. Parse & Split Command Chain  │
    │    (&&, ||, |, ;, &)            │
    │    Reject if unparseable        │
    └─────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────┐
    │ 3. For Each Segment:            │
    │    a. Check dangerous patterns  │
    │       ($() and backticks)       │
    │    b. Check deny list           │
    │    c. Strip wrappers            │
    │    d. Check deny on core cmd    │
    │    e. Check safe patterns       │
    └─────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────┐
    │ 4. Log to Audit Trail           │
    │    (all segments evaluated)     │
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

**Per-segment checking**: Dangerous patterns are checked for each segment individually after the command is parsed and split. This ensures all segments are evaluated and logged in the audit trail, even if an earlier segment contains command substitution.

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
- **All segments are evaluated** regardless of whether earlier segments are rejected (for complete audit logging)

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

### 5.5 Embedded Default Config

- Restrictive by design (fail-secure)
- Used when no config file exists
- Embedded via `go:embed`

---

## 6. CLI Interface

### 6.1 Commands

| Command | Description |
|---------|-------------|
| `mmi` (default) | Process hook input from stdin, output decision to stdout |
| `mmi init` | Create default config file and configure Claude Code settings |
| `mmi validate` | Display compiled patterns from config |
| `mmi completion` | Generate shell completions (bash, zsh, fish, powershell) |

### 6.2 Global Flags

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Enable debug logging |
| `--dry-run` | Test commands without JSON output |
| `--no-audit-log` | Disable audit logging |

### 6.3 Init Command Flags

| Flag | Description |
|------|-------------|
| `-f, --force` | Overwrite existing config file |
| `--config-only` | Only write config.toml, skip Claude settings configuration |
| `--claude-settings` | Path to Claude settings.json (default: ~/.claude/settings.json) |

### 6.4 Init Command Behavior

The `mmi init` command handles two separate tasks:
1. Config file creation
2. Claude Code settings configuration

**Config file behavior:**
- If config doesn't exist or `--force` is set: creates/overwrites config file
- If config exists and `--force` is not set: prints notice and skips writing

**Claude settings behavior:**
- Always runs unless `--config-only` is set
- Checks if mmi hook is already present
- If not present: adds hook to settings.json
- If already present: prints notice and skips

This separation allows users to reconfigure Claude Code hooks without needing `--force`, which would unnecessarily overwrite their config file.

---

## 7. Claude Code Integration

### 7.1 Hook Configuration

Running `mmi init` automatically configures Claude Code's `~/.claude/settings.json` to add the mmi hook. The configuration added:

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

**Automatic configuration behavior:**
- Preserves all existing settings in `settings.json`
- Creates `~/.claude/` directory if it doesn't exist
- Skips configuration if mmi hook is already present
- Runs independently of config file creation (unless `--config-only` is set)
- Use `--claude-settings` to specify a custom settings.json path

### 7.2 Hook Protocol

**Input** (stdin JSON from Claude Code):
```json
{
  "session_id": "abc123",
  "transcript_path": "/Users/.../.claude/projects/.../session.jsonl",
  "cwd": "/Users/...",
  "permission_mode": "default",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": {
    "command": "git status",
    "description": "Check repository status",
    "timeout": 120000
  },
  "tool_use_id": "toolu_01ABC123..."
}
```
Note: `description` and `timeout` in `tool_input` are optional; all other fields are required.

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

### 8.2 Format (v1)

JSON-lines (one JSON object per line):
```json
{
  "version": 1,
  "tool_use_id": "tool-abc123",
  "session_id": "session-xyz789",
  "timestamp": "2025-01-15T10:30:00Z",
  "duration_ms": 1.25,
  "command": "sudo git status && ls",
  "approved": true,
  "segments": [
    {
      "command": "sudo git status",
      "approved": true,
      "wrappers": ["sudo"],
      "match": {"type": "subcommand", "pattern": "^git\\s+(status|log)\\b", "name": "git"}
    },
    {
      "command": "ls",
      "approved": true,
      "match": {"type": "simple", "pattern": "^ls\\b", "name": "ls"}
    }
  ],
  "cwd": "/home/user/project",
  "input": "{...}",
  "output": "{...}",
  "config_path": "/home/user/.config/mmi/config.toml"
}
```

### 8.3 Fields

| Field | Description |
|-------|-------------|
| `version` | Log format version (currently 1) |
| `tool_use_id` | Claude Code tool use ID |
| `session_id` | Claude Code session ID |
| `timestamp` | UTC timestamp (RFC3339) |
| `duration_ms` | Processing time in milliseconds |
| `command` | The full command evaluated |
| `approved` | Boolean approval result |
| `segments` | Array of segment details |
| `cwd` | Current working directory |
| `input` | Raw JSON input from Claude Code |
| `output` | JSON output sent back to Claude Code |
| `config_path` | Path to the config file used |
| `config_error` | Config parse error message (omitted if valid) |

### 8.4 Segment Fields

| Field | Description |
|-------|-------------|
| `command` | Individual command segment |
| `approved` | Boolean approval for this segment |
| `wrappers` | Array of wrapper names stripped (omitted if empty) |
| `match` | Match details (present if approved) |
| `rejection` | Rejection details (present if rejected) |

### 8.5 Match Fields

| Field | Description |
|-------|-------------|
| `type` | Pattern type: `simple`, `subcommand`, `command`, `regex` |
| `pattern` | Regex pattern that matched (may be omitted) |
| `name` | Pattern name from config |

### 8.6 Rejection Fields

| Field | Description |
|-------|-------------|
| `code` | One of: `COMMAND_SUBSTITUTION`, `UNPARSEABLE`, `DENY_MATCH`, `NO_MATCH` |
| `name` | Deny pattern name (DENY_MATCH only) |
| `pattern` | Pattern that triggered rejection (COMMAND_SUBSTITUTION, DENY_MATCH) |
| `detail` | Error details (UNPARSEABLE only) |

### 8.7 Rejection Codes

| Code | Description | When Used |
|------|-------------|-----------|
| `COMMAND_SUBSTITUTION` | Command substitution detected | `$(...)` or backticks outside quoted heredocs |
| `UNPARSEABLE` | Shell syntax error | Incomplete syntax, unclosed quotes |
| `DENY_MATCH` | Matched deny pattern | Command matches a deny list pattern |
| `NO_MATCH` | No safe pattern matched | Command not in allowlist |

### 8.8 Migration from v0

Logs from earlier versions (implicit v0) do not have a `version` field and use a simpler format:
```json
{"timestamp":"2025-01-15T10:30:00Z","command":"git status","approved":true,"reason":"git"}
```

When parsing logs, check for the `version` field to determine format:
- No `version` field: v0 format
- `version: 1`: v1 format with segments

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
- Segment-by-segment chain validation with **complete evaluation** (all segments evaluated and logged, even after rejection)
- Wrapper stripping before core command validation
- Re-check deny patterns after wrapper stripping

### 9.3 No Bypass Paths

- Silent failures for unparseable JSON input
- Explicit rejection for unparseable shell commands
- Graceful fallback to embedded defaults if config missing

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
- Homebrew tap: `dgerlanc/homebrew-tap` (formula auto-updated on release)

### 11.3 CI/CD

**GitHub Actions Workflows**:
- `ci.yml` - Push/PR: format check, tests, build verification
- `release.yml` - Tags: GoReleaser publish + Homebrew tap update

**Required Secrets**:
- `GITHUB_TOKEN` - Automatic, used for creating GitHub releases
- `HOMEBREW_TAP_TOKEN` - PAT with `contents: write` on `dgerlanc/homebrew-tap`, used by GoReleaser to push the updated formula

---

## 12. Environment Variables

| Variable | Purpose |
|----------|---------|
| `MMI_CONFIG` | Custom config directory |

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
