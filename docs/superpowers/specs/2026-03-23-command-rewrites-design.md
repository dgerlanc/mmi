# Command Rewrites Design

## Overview

Add a command rewriting feature to MMI that rejects commands matching rewrite rules and suggests corrected alternatives. This enables enforcing preferred tooling (e.g., `uv run python` instead of `python`) by rejecting the original command with a helpful message so Claude retries with the correct command.

## Key Decisions

- **Mechanism**: Reject with suggestion (not transparent rewrite). MMI returns `"ask"` with a reason containing the suggested command. Claude then re-submits the rewritten command, which goes through the full approval pipeline from scratch. This is safe by design — rewrites are hints, not bypasses.
- **Pipeline position**: Rewrites are checked at the end of per-segment evaluation, after deny/wrapper/safe checks. Rewrites fire regardless of whether the command is safe-listed. Deny-matched and dangerous-pattern segments are skipped (no rewrite suggestion).
- **Matching target**: Rewrites match against the wrapper-stripped core command, consistent with how the safe list works.
- **Chain handling**: When a command chain has mixed approved and rewritten segments, MMI reconstructs the full corrected chain in the rejection message (e.g., `git status && uv run python script.py`).
- **Architecture**: Inline in the hook pipeline (approach 1). No new packages — rewrite logic lives in existing `hook`, `config`, and `patterns` packages.

## Configuration

New `[[rewrites.*]]` TOML section supporting simple and regex variants:

```toml
# Simple: match command names, replace prefix, preserve arguments
[[rewrites.simple]]
name = "use uv for python"
match = ["python", "python3"]
replace = "uv run python"

# Regex: match pattern with capture group support
[[rewrites.regex]]
name = "use uv for pip"
pattern = '^pip3?\b'
replace = "uv pip"
```

### Simple rewrites

- `match`: list of command names (like `commands.simple` uses `commands`)
- `replace`: replacement prefix string
- Matching uses `BuildSimplePattern(cmd)` (i.e., `^cmd\b`)
- The matched command prefix is replaced with `replace`; remaining arguments are preserved
- Example: `python3 script.py --verbose` with match `python3`, replace `uv run python` → `uv run python script.py --verbose`

### Regex rewrites

- `pattern`: Go regex (RE2 syntax) matched against the core command
- `replace`: replacement string supporting `$1`, `$2` capture groups via `Regexp.ReplaceAllString`
- The full core command is transformed by the regex replacement

### Merging

Rewrite rules from included files are merged by appending, same as other pattern types. First matching rule wins.

## New Types

### `patterns.RewriteRule`

```go
// RewriteRule holds a compiled match pattern and its replacement string.
type RewriteRule struct {
    Regex   *regexp.Regexp
    Name    string
    Type    string // "simple" or "regex"
    Pattern string // original pattern string
    Replace string // replacement string
}
```

Lives in `internal/patterns/` alongside `Pattern`. Similar to `Pattern` but adds the `Replace` field.

### `config.Config` addition

```go
type Config struct {
    // ... existing fields ...
    RewriteRules []patterns.RewriteRule
}
```

### `hook.RewriteResult`

```go
type RewriteResult struct {
    Matched     bool
    Name        string
    Pattern     string
    Replacement string // the fully rewritten core command
}
```

## Hook Pipeline Changes

### Per-segment evaluation (inside existing loop)

The segment evaluation order becomes:

1. Dangerous patterns → reject (skip rewrite)
2. Deny list on full segment → reject (skip rewrite)
3. Strip wrappers
4. Deny list on core command → reject (skip rewrite)
5. Safe list check
6. **Rewrite check on core command** (NEW — runs regardless of safe match result)
7. If rewrite matched → override result to `REWRITE` rejection

### New function: `CheckRewrite`

```go
func CheckRewrite(coreCmd string, rules []patterns.RewriteRule) RewriteResult
```

- For simple rules: if the regex matches, replace the matched prefix with `Replace` and append the remaining arguments
- For regex rules: use `Regexp.ReplaceAllString(coreCmd, rule.Replace)`
- Returns first match (rules evaluated in config order)

### Post-loop logic

If any segment triggered a rewrite (and none were deny-matched/dangerous):

1. Reconstruct the full command: for each segment, use the rewritten version if rewritten, otherwise use the original
2. Join segments with ` && `
3. Return `FormatAsk()` with reason: `rewrite: use "git status && uv run python script.py" instead`

If the chain contains both deny-matched segments and rewrites, the deny takes precedence (existing behavior — deny produces a `"deny"` decision).

## Audit Log Changes

One new rejection code:

```go
const CodeRewrite = "REWRITE"
```

Audit segment for a rewritten command:

```json
{
  "command": "python3 script.py",
  "approved": false,
  "rejection": {
    "code": "REWRITE",
    "name": "use uv for python",
    "pattern": "^python3\\b",
    "detail": "uv run python script.py"
  }
}
```

No structural changes to `Entry`, `Segment`, `Match`, or `Rejection`. The audit version stays at 1 since this is additive.

## Config Parsing

New `parseRewriteSection` function in `config.go`:

### `[[rewrites.simple]]`
- Required fields: `name` (string), `match` (string array), `replace` (string)
- For each command in `match`, builds `BuildSimplePattern(cmd)` regex
- Creates a `RewriteRule` with compiled regex and `replace` string

### `[[rewrites.regex]]`
- Required fields: `name` (string), `pattern` (string), `replace` (string)
- Compiles `pattern` directly as Go regex
- Creates a `RewriteRule` with compiled regex and `replace` string

Validation errors follow existing format: `rewrites.simple[0] "use uv": "match" field is required and must not be empty`.

Parsed in `loadConfigWithIncludes` after existing sections:

```go
if rewritesSection, ok := raw["rewrites"].(map[string]any); ok {
    rewrites, err := parseRewriteSection(rewritesSection)
    cfg.RewriteRules = append(cfg.RewriteRules, rewrites...)
}
```

## Validate Command

New section in `mmi validate` output:

```
Rewrite rules (2):
  [simple]  "use uv for python"    ^python\b → uv run python
  [simple]  "use uv for python"    ^python3\b → uv run python
  [regex]   "use uv for pip"       ^pip3?\b → uv pip
```

## Testing Strategy

### `hook_test.go`
- `CheckRewrite` with simple rules — match, no match, preserves arguments
- `CheckRewrite` with regex rules — capture groups, no match
- Rewrite overrides safe match (python is safe but gets rewritten)
- Rewrite skipped for deny-matched segments
- Rewrite skipped for dangerous-pattern segments
- Mixed chain: `git status && python3 script.py` → full corrected chain in rejection

### `config_test.go`
- Parse `[[rewrites.simple]]` — valid config
- Parse `[[rewrites.regex]]` — valid config
- Missing `match` field → error
- Missing `replace` field → error
- Invalid regex pattern → error
- Rewrite rules merge across includes

### `main_test.go` (integration)
- End-to-end: command with rewrite rule → JSON output contains `"ask"` with rewrite suggestion
- End-to-end: command chain with partial rewrite → full corrected chain in reason
- Audit log entry contains `REWRITE` rejection code with correct detail
