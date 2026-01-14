# Plan: Update Audit Log Format

This plan follows test-first development. Each phase begins with writing failing tests, then implementing code to pass them.

## Structs

```go
type Entry struct {
    Version    int       `json:"version"`
    ToolUseID  string    `json:"tool_use_id"`
    SessionID  string    `json:"session_id,omitempty"`
    Timestamp  time.Time `json:"timestamp"`
    DurationMs float64   `json:"duration_ms,omitempty"`
    Command    string    `json:"command"`
    Approved   bool      `json:"approved"`
    Segments   []Segment `json:"segments"`
    Cwd        string    `json:"cwd,omitempty"`
    Profile    string    `json:"profile,omitempty"`
}

type Segment struct {
    Command   string     `json:"command"`
    Approved  bool       `json:"approved"`
    Match     *Match     `json:"match,omitempty"`
    Rejection *Rejection `json:"rejection,omitempty"`
}

type Match struct {
    Type    string `json:"type,omitempty"`
    Pattern string `json:"pattern,omitempty"`
    Name    string `json:"name"`
    Layer   string `json:"layer"`
}

type Rejection struct {
    Code           string `json:"code"`
    Detail         string `json:"detail,omitempty"`
    Pattern        string `json:"pattern,omitempty"`
    MatchedCommand string `json:"matched_command,omitempty"`
    Layer          string `json:"layer,omitempty"`
}
```

## Rejection Codes

| Code | Description | When Used |
|------|-------------|-----------|
| `COMMAND_SUBSTITUTION` | Command substitution detected | `$(...)` or backticks outside quoted heredocs |
| `UNPARSEABLE` | Shell syntax error | Incomplete syntax, unclosed quotes |
| `DENY_MATCH` | Matched deny pattern | Command matches a deny list pattern |
| `NO_MATCH` | No safe pattern matched | Command not in allowlist |

Example rejections:

```json
{"code": "COMMAND_SUBSTITUTION", "detail": "command substitution", "pattern": "$(...)"}
{"code": "UNPARSEABLE", "detail": "unclosed quote"}
{"code": "DENY_MATCH", "detail": "privilege escalation", "pattern": "^sudo\\b", "layer": "deny"}
{"code": "NO_MATCH", "detail": "no matching safe pattern", "matched_command": "curl"}
```

## Phase 1: Entry Serialization

### 1a. Write tests in `internal/audit/audit_test.go`

- Test `Entry` JSON serialization with all fields
- Test `Entry` JSON serialization with omitempty fields empty
- Test `Segment` with `Match` serializes correctly
- Test `Segment` with `Rejection` serializes correctly for each rejection code:
  - `COMMAND_SUBSTITUTION`
  - `UNPARSEABLE`
  - `DENY_MATCH`
  - `NO_MATCH`
- Test version field is always present

### 1b. Implement structs

- Add `Entry`, `Segment`, `Match`, `Rejection` structs to `internal/audit/audit.go`
- Add rejection code constants:
  ```go
  const (
      CodeCommandSubstitution = "COMMAND_SUBSTITUTION"
      CodeUnparseable         = "UNPARSEABLE"
      CodeDenyMatch           = "DENY_MATCH"
      CodeNoMatch             = "NO_MATCH"
  )
  ```
- Run tests to verify serialization

## Phase 2: Session and ToolUseID Tracking

Both `SessionID` and `ToolUseID` are already available in the `Input` struct from Claude Code.

### 2a. Write tests in `internal/hook/hook_test.go`

- Test `ProcessWithResult()` passes session ID to audit log
- Test `ProcessWithResult()` passes tool use ID to audit log
- Test both IDs appear in logged audit entry

### 2b. Pass IDs to audit

- Update `logAudit()` to accept and pass `sessionID` and `toolUseID` to audit entry
- Run tests to verify

## Phase 3: Pattern Match Results

### 3a. Write tests in `internal/hook/hook_test.go`

- Test `CheckSafe()` returns `SafeResult` with match metadata
- Test `CheckSafe()` returns pattern name, type, and regex
- Test `CheckDeny()` returns `DenyResult` with rejection metadata
- Test `CheckDeny()` returns pattern name and regex

### 3b. Update pattern matching functions

```go
type SafeResult struct {
    Matched bool
    Name    string
    Type    string   // simple, subcommand, regex, command
    Pattern string
}

func CheckSafe(cmd string) SafeResult

type DenyResult struct {
    Denied  bool
    Name    string
    Pattern string
}

func CheckDeny(cmd string) DenyResult
```

- Run tests to verify

## Phase 4: Hook Integration

### 4a. Write tests in `internal/hook/hook_test.go`

- Test `ProcessWithResult()` populates segments for single command
- Test `ProcessWithResult()` populates segments for chained commands
- Test approved segment has `Match` populated
- Test rejected segment has `Rejection` populated with correct code:
  - `COMMAND_SUBSTITUTION` for `$(...)` or backticks
  - `UNPARSEABLE` for syntax errors
  - `DENY_MATCH` for deny list matches
  - `NO_MATCH` for commands not in allowlist
- Test `DurationMs` is greater than 0

### 4b. Update hook integration

- Update `logAudit()` signature:
  ```go
  func logAudit(command string, approved bool, segments []audit.Segment, durationMs float64)
  ```
- Track timing in `ProcessWithResult()`
- Build `Segment` slice during command evaluation
- Run tests to verify

## Phase 5: Log Function Update

### 5a. Write tests in `internal/audit/audit_test.go`

- Test `Log()` writes entry with version field
- Test `Log()` writes entry with tool use ID field
- Test `Log()` writes entry with session ID
- Test `Log()` writes entry with segments
- Test `Log()` writes entry with cwd when provided

### 5b. Update `Log()` function

- Set `Version` to 1
- Tool use ID and session ID passed from caller (from `input.ToolUseID` and `input.SessionID`)
- Run tests to verify

## Phase 6: Documentation

- Update `docs/SPEC.md` section 8 with new format
- Update `docs/AUDIT_FORMAT_SUGGESTIONS.md` to mark implemented items
- Add migration notes for users parsing old logs

## File Changes Summary

| File | Changes |
|------|---------|
| `internal/audit/audit.go` | Add structs, update `Entry` |
| `internal/audit/audit_test.go` | Add tests for new structs |
| `internal/hook/hook.go` | Update `logAudit()`, add timing, build segments, pass IDs and cwd |
| `internal/hook/hook_test.go` | Add tests for IDs in audit, match results, segments |
| `docs/SPEC.md` | Update audit format documentation |

## Backward Compatibility

- Version field distinguishes old (implicit v0) from new (v1) entries
- Existing logs remain readable
- Consider adding `mmi audit migrate` command for converting old logs
