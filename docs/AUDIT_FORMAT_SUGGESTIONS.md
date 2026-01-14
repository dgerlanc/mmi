# Audit Format Improvement Suggestions

This document outlines suggestions for enhancing the MMI audit log format to improve traceability, analysis capabilities, and operational robustness.

## Current Format

The current audit format is JSON-lines with these fields:

```json
{"timestamp":"2025-01-15T10:30:00Z","command":"git status","approved":true,"reason":"git","profile":""}
```

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | RFC3339 UTC timestamp |
| `command` | string | The command evaluated |
| `approved` | bool | Approval decision |
| `reason` | string | Pattern name or rejection reason (omitempty) |
| `profile` | string | Configuration profile used (omitempty) |

---

## Suggested Improvements

### 1. Schema Versioning

**Problem**: Future changes to the audit format may break tooling that parses audit logs.

**Suggestion**: Add a `version` field to enable format evolution.

```json
{"version":1,"timestamp":"2025-01-15T10:30:00Z","command":"git status","approved":true,"reason":"git"}
```

**Implementation**:
```go
type Entry struct {
    Version   int       `json:"version"`
    // ... existing fields
}
```

**Benefits**:
- Parsers can handle multiple format versions
- Enables backward-compatible format changes
- Clear migration path for tooling

---

### 2. Unique Entry Identifier

**Problem**: No way to reference specific audit entries for investigation or correlation.

**Suggestion**: Add a unique `id` field using ULID or UUID.

```json
{"id":"01HQXYZ...","timestamp":"2025-01-15T10:30:00Z","command":"git status",...}
```

**Implementation Options**:
- **ULID**: Sortable, timestamp-embedded (recommended)
- **UUID v7**: Time-ordered UUID
- **Sequence number**: Simple incrementing counter per session

**Benefits**:
- Reference specific entries in reports/tickets
- Correlate with external systems
- Deduplication detection

---

### 3. Session Tracking

**Problem**: Cannot group related commands from a single Claude Code session.

**Suggestion**: Add `session_id` to correlate commands from the same session.

```json
{"session_id":"abc123","timestamp":"...","command":"git status",...}
{"session_id":"abc123","timestamp":"...","command":"git diff",...}
```

**Implementation**:
- Generate session ID on first invocation
- Pass via environment variable or temp file
- Claude Code could provide this via hook context

**Benefits**:
- Analyze command patterns per session
- Debug issues in specific sessions
- Usage analytics per session

---

### 4. Execution Context

**Problem**: No context about where/how commands were executed.

**Suggestion**: Add optional context fields:

```json
{
  "timestamp": "...",
  "command": "git status",
  "context": {
    "hostname": "dev-machine",
    "user": "developer",
    "cwd": "/home/dev/project",
    "pid": 12345
  },
  ...
}
```

**Implementation**:
```go
type Context struct {
    Hostname string `json:"hostname,omitempty"`
    User     string `json:"user,omitempty"`
    Cwd      string `json:"cwd,omitempty"`
    PID      int    `json:"pid,omitempty"`
}
```

**Benefits**:
- Multi-machine audit aggregation
- Forensic analysis
- Understanding command context

---

### 5. Pattern Match Details

**Problem**: `reason` field only shows pattern name, not which specific pattern/rule matched.

**Suggestion**: Add structured match information:

```json
{
  "command": "git status",
  "approved": true,
  "match": {
    "type": "subcommand",
    "pattern": "^git\\s+(status|log|diff)\\b",
    "name": "git",
    "layer": "safe_commands"
  }
}
```

**For rejections**:
```json
{
  "command": "sudo rm -rf /",
  "approved": false,
  "match": {
    "type": "simple",
    "name": "privilege escalation",
    "layer": "deny",
    "matched_command": "sudo"
  }
}
```

**Benefits**:
- Debug pattern matching issues
- Audit pattern effectiveness
- Optimize patterns based on usage

---

### 6. Command Chain Details

**Problem**: For chained commands (`&&`, `||`, `|`), only the full command is logged.

**Suggestion**: Optionally log segment-level decisions:

```json
{
  "command": "git status && npm install",
  "approved": true,
  "segments": [
    {"command": "git status", "approved": true, "reason": "git"},
    {"command": "npm install", "approved": true, "reason": "npm"}
  ]
}
```

**Benefits**:
- Understand which segment caused rejection
- Audit complex command chains
- Better debugging for chain failures

---

### 7. Timing Information

**Problem**: No insight into processing performance.

**Suggestion**: Add processing duration:

```json
{
  "timestamp": "2025-01-15T10:30:00.000Z",
  "duration_ms": 2.5,
  "command": "git status",
  ...
}
```

**Benefits**:
- Performance monitoring
- Identify slow patterns
- Optimization opportunities

---

### 8. Structured Rejection Reasons

**Problem**: Rejection reasons are free-form strings making automated analysis difficult.

**Suggestion**: Use structured rejection codes:

```json
{
  "command": "$(cat /etc/passwd)",
  "approved": false,
  "rejection": {
    "code": "DANGEROUS_PATTERN",
    "detail": "command substitution",
    "pattern": "$(...)"
  }
}
```

**Rejection Codes**:
| Code | Description |
|------|-------------|
| `DANGEROUS_PATTERN` | Command substitution detected |
| `UNPARSEABLE` | Shell syntax error |
| `DENY_MATCH` | Matched deny pattern |
| `NOT_SAFE` | No safe pattern matched |
| `NON_BASH_TOOL` | Tool is not Bash |

**Benefits**:
- Programmatic rejection analysis
- Metrics on rejection types
- Alert on specific rejection categories

---

### 9. Log Rotation Support

**Problem**: Audit log grows unbounded.

**Suggestions**:

1. **Built-in rotation**:
   - Rotate when file exceeds size threshold (e.g., 10MB)
   - Keep N rotated files (e.g., `audit.log.1`, `audit.log.2`)
   - Optional compression for rotated files

2. **External rotation compatibility**:
   - Handle SIGHUP to reopen log file
   - Document logrotate configuration

3. **Date-based logs**:
   - Option to use date-based filenames: `audit-2025-01-15.log`
   - Automatic daily rotation

**Configuration**:
```toml
[audit]
path = "~/.local/share/mmi/audit.log"
max_size_mb = 10
max_files = 5
compress = true
```

---

### 10. Output Format Options

**Problem**: JSON-lines may not suit all analysis needs.

**Suggestions**:

1. **CSV export**:
   ```bash
   mmi audit export --format csv > audit.csv
   ```

2. **SQLite integration**:
   - Option to log directly to SQLite
   - Better querying capabilities
   - Built-in rotation via VACUUM

3. **Syslog integration**:
   - Forward to syslog for enterprise logging
   - Integration with SIEM systems

---

### 11. Integrity Verification

**Problem**: No way to verify audit log hasn't been tampered with.

**Suggestions**:

1. **Entry hashing**:
   ```json
   {"timestamp":"...","command":"git status","hash":"sha256:abc123..."}
   ```

2. **Chain hashing**:
   - Each entry includes hash of previous entry
   - Detects insertions/deletions/modifications

3. **Periodic checksums**:
   - Write checksum entries periodically
   - Verify integrity on demand

---

### 12. Summary Statistics

**Problem**: No built-in way to analyze audit data.

**Suggestion**: Add `mmi audit stats` command:

```bash
$ mmi audit stats --since 7d

Audit Summary (last 7 days)
===========================
Total commands:     1,234
Approved:           1,180 (95.6%)
Rejected:              54 (4.4%)

Top approved patterns:
  git: 456 (38.6%)
  npm: 234 (19.8%)
  ls:  123 (10.4%)

Rejection breakdown:
  privilege escalation: 32 (59.3%)
  dangerous pattern:    15 (27.8%)
  unsafe command:        7 (13.0%)
```

---

## Proposed Enhanced Entry Format

Combining suggestions, the enhanced format could be:

```json
{
  "version": 1,
  "id": "01HQXYZ123456789",
  "session_id": "sess_abc123",
  "timestamp": "2025-01-15T10:30:00.123Z",
  "duration_ms": 2.5,
  "command": "git status && npm test",
  "approved": true,
  "match": {
    "type": "subcommand",
    "name": "git",
    "layer": "safe_commands"
  },
  "context": {
    "hostname": "dev-machine",
    "cwd": "/home/dev/project"
  },
  "profile": "python"
}
```

**Minimal format (backward compatible)**:
```json
{"version":1,"timestamp":"2025-01-15T10:30:00Z","command":"git status","approved":true,"reason":"git"}
```

---

## Implementation Priority

| Priority | Suggestion | Effort | Impact |
|----------|------------|--------|--------|
| High | Schema versioning | Low | High |
| High | Structured rejection codes | Medium | High |
| Medium | Unique entry ID | Low | Medium |
| Medium | Session tracking | Medium | Medium |
| Medium | Log rotation | Medium | High |
| Low | Timing information | Low | Low |
| Low | Pattern match details | Medium | Medium |
| Low | Command chain details | High | Medium |
| Low | Execution context | Low | Low |
| Low | Integrity verification | High | Low |

---

## Migration Strategy

1. **Phase 1**: Add `version` field (v1 = current format)
2. **Phase 2**: Add new optional fields with `omitempty`
3. **Phase 3**: Provide migration tool for old logs
4. **Phase 4**: Document format versions and changes

---

## Backward Compatibility

All suggestions maintain backward compatibility by:
- Adding `version` field to identify format
- Using `omitempty` for new optional fields
- Keeping existing field names and semantics
- Providing tools to upgrade old log formats
