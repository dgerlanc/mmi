# Subshell Command Substitution Allowlist

## Problem

MMI blanket-rejects all `$(...)` and backtick command substitution as dangerous.
This blocks legitimate use cases like multi-line commit messages and PR bodies:

```bash
git commit -m "$(cat <<'EOF'
Fix auth bug in session handler
EOF
)"
```

Claude Code frequently generates commands in this format. Users must manually
approve every one of them, defeating the purpose of mmi.

## Design

Two-stage approach. Stage 1 is the immediate deliverable; Stage 2 is a planned
follow-up.

### Stage 1: `allow_all` toggle

A new optional `[subshell]` config section with a single boolean field:

```toml
[subshell]
allow_all = false  # default; set true to permit all $() and backticks
```

**Behavior:**

| `allow_all` | Effect |
|---|---|
| `false` (default) | Current behavior — all `$(...)` and backticks rejected |
| `true` | `containsDangerousPattern()` is skipped; commands proceed to deny/safe checks |

When `allow_all = true`, the deny → wrapper → safe pipeline still runs on the
outer command. The toggle only disables the command-substitution rejection; it
does not bypass any other security checks.

**Config changes:**

- Add `SubshellConfig` struct with `AllowAll bool` field to `internal/config/`
- Add `Subshell SubshellConfig` to the top-level `Config` struct
- `[subshell]` section is optional; omitting it preserves current behavior

**Hook changes:**

- In per-segment evaluation, check `config.Subshell.AllowAll` before calling
  `containsDangerousPattern()`
- If `true`, skip the dangerous pattern check entirely for that segment

**Default config:**

- Add commented-out `[subshell]` section to `internal/config/config.toml`
- Default remains `allow_all = false`

**Audit logging:**

- No new fields in Stage 1
- Segments are approved/rejected through the normal pipeline
- The active config path is already logged in audit entries

**Tests:**

- Config parsing with and without `[subshell]` section
- Segment with `$(...)` rejected when `allow_all = false` (existing behavior)
- Segment with `$(...)` approved when `allow_all = true` and outer command
  passes deny/safe checks
- Segment with backticks approved when `allow_all = true`
- Deny patterns still reject even when `allow_all = true`

### Stage 2: `allowed_commands` allowlist (future)

Adds fine-grained control over which commands are permitted inside `$(...)`.

```toml
[subshell]
allow_all = false
allowed_commands = ["cat"]
```

**Behavior matrix:**

| `allow_all` | `allowed_commands` | Result |
|---|---|---|
| `false` | `[]` (default) | All substitution rejected |
| `false` | `["cat"]` | Only `$(cat ...)` permitted |
| `true` | (ignored) | All substitution permitted |

**AST-based extraction:**

Uses `mvdan.cc/sh/v3/syntax` to walk the AST for `*syntax.CmdSubst` nodes.
For each node, extracts the first word (command name) and checks it against
`allowed_commands`.

A new `checkSubshells()` function runs before `containsDangerousPattern()`.
It returns:

- A list of approved subshell command names (for audit)
- Whether any disallowed substitution was found

Approved `CmdSubst` byte ranges are passed to `containsDangerousPattern()`
so it skips those ranges in its regex scan. This is defensive — if a
substitution escapes the AST but matches the regex, it is still caught.

**Heredoc interaction:**

- Quoted heredocs (`<<'EOF'`): content is literal. The parser produces no
  nested `CmdSubst` nodes. No special handling needed.
- Unquoted heredocs (`<<EOF`): the parser produces `CmdSubst` nodes for any
  `$(...)` in the body. Each is checked independently against `allowed_commands`.
- No recursive/nesting support beyond what the AST naturally provides.

**Audit logging:**

New `subshell_commands` field on segment entries:

```json
{
  "command": "git commit -m \"$(cat <<'EOF'\nFix bug\nEOF\n)\"",
  "approved": true,
  "subshell_commands": ["cat"],
  "match": {
    "type": "subcommand",
    "pattern": "^git\\s+(commit)\\b",
    "name": "git"
  }
}
```

- `[]string` listing command names found and approved inside `$(...)`
- Omitted from JSON when empty (consistent with `wrappers` field)
- Populated even when `allow_all = true` for visibility
- Duplicates included (two `$(cat ...)` → `["cat", "cat"]`)

## What's NOT in scope

- No changes to the deny/wrapper/safe pattern pipeline
- No changes to how quoted heredoc content is treated (already literal)
- No recursive validation of subshell contents beyond command name extraction
