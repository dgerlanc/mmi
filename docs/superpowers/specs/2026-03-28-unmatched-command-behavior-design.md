# Unmatched Command Behavior Design

## Summary

Add a configurable `unmatched` setting that controls what MMI does when a command doesn't match any safe pattern. Currently MMI always returns `"ask"` for unmatched commands. This feature adds two additional options: `"passthrough"` (MMI abstains, letting Claude Code use its own permission logic) and `"reject"` (MMI denies the command outright).

## Motivation

Users who trust Claude Code's built-in permission system may want MMI to only act as a deny list and safe list — approving known-safe commands and blocking known-dangerous ones — without intercepting everything else. The passthrough option lets MMI step aside for commands it has no opinion on.

## Configuration

A new top-level `[defaults]` section in `config.toml`:

```toml
[defaults]
unmatched = "ask"  # "ask" (default), "passthrough", or "reject"
```

### Values

- **`"ask"`** (default) — Returns `permissionDecision: "ask"` with reason "command not in allow list". This is the current behavior.
- **`"passthrough"`** — MMI exits 0 with no output. Claude Code treats this as "no opinion" and falls back to its own permission logic.
- **`"reject"`** — Returns `permissionDecision: "deny"` with reason "command not in allow list".

### Defaults and backward compatibility

- If `[defaults]` is omitted or `unmatched` is not set, the behavior is `"ask"` (preserves current behavior).
- Invalid values (e.g. `unmatched = "foo"`) produce a config parse error.

### Include semantics

Same as `SubshellAllowAll` — last value wins. If an included file omits `[defaults]`, its zero value (empty string) is treated as `"ask"`.

## Config Struct Changes

Add an `Unmatched string` field to the `Config` struct:

```go
type Config struct {
    WrapperPatterns  []patterns.Pattern
    SafeCommands     []patterns.Pattern
    DenyPatterns     []patterns.Pattern
    SubshellAllowAll bool
    RewriteRules     []patterns.RewriteRule
    Unmatched        string // "ask" (default), "passthrough", "reject"
}
```

Parsing: read `[defaults]` section in `LoadConfig`, extract `unmatched` string value. Validate it is one of the three allowed values or empty. Empty/missing defaults to `"ask"`.

## Hook Processing Changes

The evaluation pipeline is unchanged — deny, dangerous patterns, wrappers, rewrites, and safe checks all run identically. Only the final decision for `CodeNoMatch` segments changes based on `cfg.Unmatched`:

| `unmatched` value | Output for unmatched commands |
|---|---|
| `"ask"` | JSON with `permissionDecision: "ask"`, reason "command not in allow list" |
| `"passthrough"` | Empty output (exit 0, no JSON) |
| `"reject"` | JSON with `permissionDecision: "deny"`, reason "command not in allow list" |

### Deny and rewrite override

Deny list matches and rewrite matches always produce their normal deny output regardless of the `unmatched` setting. The `unmatched` setting only affects the `CodeNoMatch` path.

### Result struct

Add a `Passthrough bool` field to `Result`. The caller (`cmd/run.go`) checks this — if true, it writes nothing to stdout and exits 0.

## Audit Logging

Passthrough commands are still logged to the audit trail. A new rejection code `PASSTHROUGH` is added to the audit package:

```go
const CodePassthrough = "PASSTHROUGH"
```

When `unmatched = "passthrough"`, segments that would have been `CodeNoMatch` are logged with `CodePassthrough` instead. The audit entry's `Output` field is empty string (reflecting the actual empty output). The `Approved` field on the entry stays `false` since MMI didn't approve the command — it abstained.

## Validate Command

The validate command displays the current `unmatched` setting at the top of the output, before other settings:

```
Configuration valid!

Unmatched command behavior: ask
Subshell allow all: false

Deny patterns: 5
  ...
```

If the value is empty (not set in config), display `"ask"` since that's the default.

## Testing

1. **Config parsing** — Verify `[defaults] unmatched` is parsed correctly for all three values (`"ask"`, `"passthrough"`, `"reject"`), and that missing/empty defaults to `"ask"`.
2. **Invalid config value** — `unmatched = "foo"` returns a config parse error.
3. **Hook processing (ask)** — Unmatched command with `unmatched = "ask"` produces JSON with `permissionDecision: "ask"`.
4. **Hook processing (passthrough)** — Unmatched command with `unmatched = "passthrough"` produces empty output and `Result.Passthrough = true`.
5. **Hook processing (reject)** — Unmatched command with `unmatched = "reject"` produces JSON with `permissionDecision: "deny"`.
6. **Deny still overrides passthrough** — With `unmatched = "passthrough"`, a deny-listed command still returns deny.
7. **Rewrite still overrides passthrough** — Rewrites still fire regardless of `unmatched` setting.
8. **Audit logging** — Passthrough commands logged with `PASSTHROUGH` rejection code.
9. **Validate output** — Shows `Unmatched command behavior` at the top of output.
