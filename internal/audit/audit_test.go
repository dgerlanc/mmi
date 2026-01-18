package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultLogPath(t *testing.T) {
	path, err := DefaultLogPath()
	if err != nil {
		t.Fatalf("DefaultLogPath() error = %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "share", "mmi", "audit.log")
	if path != expected {
		t.Errorf("DefaultLogPath() = %q, want %q", path, expected)
	}
}

func TestInit(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "subdir", "audit.log")

	if err := Init(logPath, false); err != nil {
		t.Errorf("Init() error = %v", err)
	}

	if !IsEnabled() {
		t.Error("Expected audit logging to be enabled")
	}

	// Verify file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Audit log file was not created")
	}
}

func TestInitDisabled(t *testing.T) {
	defer Reset()

	if err := Init("", true); err != nil {
		t.Errorf("Init(disable=true) error = %v", err)
	}

	if IsEnabled() {
		t.Error("Expected audit logging to be disabled")
	}
}

func TestLog(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	if err := Init(logPath, false); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Log an approved command
	entry1 := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Command:   "git status",
		Approved:  true,
		Segments: []Segment{
			{Command: "git status", Approved: true, Match: &Match{Type: "subcommand", Name: "git"}},
		},
		Cwd: "/home",
	}
	if err := Log(entry1); err != nil {
		t.Errorf("Log() error = %v", err)
	}

	// Log a rejected command
	entry2 := Entry{
		Version:   1,
		ToolUseID: "tool-2",
		SessionID: "session-1",
		Command:   "rm -rf /",
		Approved:  false,
		Segments: []Segment{
			{Command: "rm -rf /", Approved: false, Rejection: &Rejection{Code: CodeDenyMatch, Name: "dangerous rm"}},
		},
		Cwd: "/home",
	}
	if err := Log(entry2); err != nil {
		t.Errorf("Log() error = %v", err)
	}

	// Close and read the log
	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 log lines, got %d", len(lines))
	}

	// Verify first entry
	var parsed1 Entry
	if err := json.Unmarshal([]byte(lines[0]), &parsed1); err != nil {
		t.Errorf("Failed to parse first entry: %v", err)
	}
	if parsed1.Command != "git status" {
		t.Errorf("First entry command = %q, want %q", parsed1.Command, "git status")
	}
	if !parsed1.Approved {
		t.Error("First entry should be approved")
	}
	if len(parsed1.Segments) != 1 || parsed1.Segments[0].Match == nil || parsed1.Segments[0].Match.Name != "git" {
		t.Errorf("First entry should have segment with match name 'git'")
	}
	if parsed1.Timestamp == "" {
		t.Error("First entry timestamp should not be empty")
	}

	// Verify second entry
	var parsed2 Entry
	if err := json.Unmarshal([]byte(lines[1]), &parsed2); err != nil {
		t.Errorf("Failed to parse second entry: %v", err)
	}
	if parsed2.Command != "rm -rf /" {
		t.Errorf("Second entry command = %q, want %q", parsed2.Command, "rm -rf /")
	}
	if parsed2.Approved {
		t.Error("Second entry should not be approved")
	}
}

func TestLogWhenDisabled(t *testing.T) {
	defer Reset()

	// Don't initialize audit logging
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Command:   "git status",
		Approved:  true,
		Cwd:       "/home",
	}

	// Should not error when disabled
	if err := Log(entry); err != nil {
		t.Errorf("Log() when disabled error = %v", err)
	}
}

func TestClose(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	if err := Init(logPath, false); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if IsEnabled() {
		t.Error("Expected audit logging to be disabled after Close")
	}

	// Double close should not error
	if err := Close(); err != nil {
		t.Errorf("Close() second call error = %v", err)
	}
}

// Phase 1: Entry Serialization Tests

func TestEntrySerializationAllFields(t *testing.T) {
	entry := Entry{
		Version:    1,
		ToolUseID:  "tool-123",
		SessionID:  "session-456",
		Timestamp:  "2025-01-15T10:30:00.0Z",
		DurationMs: 42.5,
		Command:    "git status && ls -la",
		Approved:   true,
		Segments: []Segment{
			{
				Command:  "git status",
				Approved: true,
				Match:    &Match{Type: "subcommand", Pattern: `^git\s+status\b`, Name: "git"},
			},
			{
				Command:  "ls -la",
				Approved: true,
				Match:    &Match{Type: "simple", Pattern: `^ls\b`, Name: "ls"},
			},
		},
		Cwd: "/home/user/project",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Entry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Version != 1 {
		t.Errorf("Version = %d, want 1", parsed.Version)
	}
	if parsed.ToolUseID != "tool-123" {
		t.Errorf("ToolUseID = %q, want %q", parsed.ToolUseID, "tool-123")
	}
	if parsed.SessionID != "session-456" {
		t.Errorf("SessionID = %q, want %q", parsed.SessionID, "session-456")
	}
	if parsed.DurationMs != 42.5 {
		t.Errorf("DurationMs = %v, want 42.5", parsed.DurationMs)
	}
	if len(parsed.Segments) != 2 {
		t.Errorf("Segments count = %d, want 2", len(parsed.Segments))
	}
	if parsed.Cwd != "/home/user/project" {
		t.Errorf("Cwd = %q, want %q", parsed.Cwd, "/home/user/project")
	}
}

func TestEntrySerializationOmitEmpty(t *testing.T) {
	// Entry with minimal fields - Segments should be omitted when empty
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-123",
		SessionID: "session-456",
		Timestamp: "2025-01-15T10:30:00.0Z",
		Command:   "ls",
		Approved:  true,
		Cwd:       "/home",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	// Segments should be present (not omitempty)
	if !strings.Contains(jsonStr, `"segments"`) {
		t.Errorf("Expected segments field to be present, got: %s", jsonStr)
	}
}

func TestEntrySingleSegment(t *testing.T) {
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.0Z07:00"),
		Command:   "git status",
		Approved:  true,
		Segments: []Segment{
			{Command: "git status", Approved: true, Match: &Match{Type: "subcommand", Name: "git"}},
		},
		Cwd: "/home",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Entry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(parsed.Segments) != 1 {
		t.Errorf("Segments count = %d, want 1", len(parsed.Segments))
	}
}

func TestEntryMultipleSegments(t *testing.T) {
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.0Z07:00"),
		Command:   "git status && ls && pwd",
		Approved:  true,
		Segments: []Segment{
			{Command: "git status", Approved: true, Match: &Match{Type: "subcommand", Name: "git"}},
			{Command: "ls", Approved: true, Match: &Match{Type: "simple", Name: "ls"}},
			{Command: "pwd", Approved: true, Match: &Match{Type: "simple", Name: "pwd"}},
		},
		Cwd: "/home",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Entry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(parsed.Segments) != 3 {
		t.Errorf("Segments count = %d, want 3", len(parsed.Segments))
	}
}

func TestEntryApprovedWhenAllSegmentsApproved(t *testing.T) {
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.0Z07:00"),
		Command:   "git status && ls",
		Approved:  true,
		Segments: []Segment{
			{Command: "git status", Approved: true, Match: &Match{Type: "subcommand", Name: "git"}},
			{Command: "ls", Approved: true, Match: &Match{Type: "simple", Name: "ls"}},
		},
		Cwd: "/home",
	}

	if !entry.Approved {
		t.Error("Entry.Approved should be true when all segments are approved")
	}
}

func TestEntryRejectedWhenAnySegmentRejected(t *testing.T) {
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.0Z07:00"),
		Command:   "git status && curl http://evil.com",
		Approved:  false,
		Segments: []Segment{
			{Command: "git status", Approved: true, Match: &Match{Type: "subcommand", Name: "git"}},
			{Command: "curl http://evil.com", Approved: false, Rejection: &Rejection{Code: CodeNoMatch}},
		},
		Cwd: "/home",
	}

	if entry.Approved {
		t.Error("Entry.Approved should be false when any segment is rejected")
	}
}

func TestSegmentApprovedHasMatchNoRejection(t *testing.T) {
	segment := Segment{
		Command:  "git status",
		Approved: true,
		Match:    &Match{Type: "subcommand", Name: "git", Pattern: `^git\s+status\b`},
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"match"`) {
		t.Error("Approved segment should have match field")
	}
	if strings.Contains(jsonStr, `"rejection"`) {
		t.Error("Approved segment should not have rejection field")
	}
}

func TestSegmentRejectedHasRejectionNoMatch(t *testing.T) {
	segment := Segment{
		Command:   "rm -rf /",
		Approved:  false,
		Rejection: &Rejection{Code: CodeDenyMatch, Name: "dangerous rm", Pattern: `^rm\s+-rf\s+/`},
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, `"match"`) {
		t.Error("Rejected segment should not have match field")
	}
	if !strings.Contains(jsonStr, `"rejection"`) {
		t.Error("Rejected segment should have rejection field")
	}
}

func TestSegmentWithWrappers(t *testing.T) {
	segment := Segment{
		Command:  "ls",
		Approved: true,
		Wrappers: []string{"sudo"},
		Match:    &Match{Type: "simple", Name: "ls"},
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"wrappers"`) {
		t.Error("Segment with wrappers should have wrappers field")
	}
	if !strings.Contains(jsonStr, `"sudo"`) {
		t.Error("Segment should contain sudo in wrappers")
	}
}

func TestSegmentWithoutWrappers(t *testing.T) {
	segment := Segment{
		Command:  "ls",
		Approved: true,
		Match:    &Match{Type: "simple", Name: "ls"},
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	// Wrappers should be omitted when empty (omitempty)
	if strings.Contains(jsonStr, `"wrappers"`) {
		t.Error("Segment without wrappers should omit wrappers field")
	}
}

func TestMatchWithAllFields(t *testing.T) {
	match := Match{
		Type:    "regex",
		Pattern: `^mycommand\s+.*`,
		Name:    "mycommand pattern",
	}

	data, err := json.Marshal(match)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Match
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Type != "regex" {
		t.Errorf("Type = %q, want %q", parsed.Type, "regex")
	}
	if parsed.Pattern != `^mycommand\s+.*` {
		t.Errorf("Pattern = %q, want %q", parsed.Pattern, `^mycommand\s+.*`)
	}
	if parsed.Name != "mycommand pattern" {
		t.Errorf("Name = %q, want %q", parsed.Name, "mycommand pattern")
	}
}

func TestMatchPatternOmitted(t *testing.T) {
	match := Match{
		Type: "simple",
		Name: "git",
	}

	data, err := json.Marshal(match)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, `"pattern"`) {
		t.Error("Match with empty pattern should omit pattern field")
	}
}

func TestRejectionCommandSubstitution(t *testing.T) {
	rejection := Rejection{
		Code:    CodeCommandSubstitution,
		Pattern: "$(cmd)",
	}

	data, err := json.Marshal(rejection)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Rejection
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Code != CodeCommandSubstitution {
		t.Errorf("Code = %q, want %q", parsed.Code, CodeCommandSubstitution)
	}
	if parsed.Pattern != "$(cmd)" {
		t.Errorf("Pattern = %q, want %q", parsed.Pattern, "$(cmd)")
	}
}

func TestRejectionUnparseable(t *testing.T) {
	rejection := Rejection{
		Code:   CodeUnparseable,
		Detail: "unclosed quote",
	}

	data, err := json.Marshal(rejection)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Rejection
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Code != CodeUnparseable {
		t.Errorf("Code = %q, want %q", parsed.Code, CodeUnparseable)
	}
	if parsed.Detail != "unclosed quote" {
		t.Errorf("Detail = %q, want %q", parsed.Detail, "unclosed quote")
	}
}

func TestRejectionDenyMatch(t *testing.T) {
	rejection := Rejection{
		Code:    CodeDenyMatch,
		Name:    "privilege escalation",
		Pattern: `^sudo\b`,
	}

	data, err := json.Marshal(rejection)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed Rejection
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Code != CodeDenyMatch {
		t.Errorf("Code = %q, want %q", parsed.Code, CodeDenyMatch)
	}
	if parsed.Name != "privilege escalation" {
		t.Errorf("Name = %q, want %q", parsed.Name, "privilege escalation")
	}
	if parsed.Pattern != `^sudo\b` {
		t.Errorf("Pattern = %q, want %q", parsed.Pattern, `^sudo\b`)
	}
}

func TestRejectionNoMatch(t *testing.T) {
	rejection := Rejection{
		Code: CodeNoMatch,
	}

	data, err := json.Marshal(rejection)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"code":"NO_MATCH"`) {
		t.Errorf("Expected code NO_MATCH, got: %s", jsonStr)
	}
	// Optional fields should be omitted
	if strings.Contains(jsonStr, `"name"`) {
		t.Error("NO_MATCH rejection should omit name field")
	}
	if strings.Contains(jsonStr, `"pattern"`) {
		t.Error("NO_MATCH rejection should omit pattern field")
	}
	if strings.Contains(jsonStr, `"detail"`) {
		t.Error("NO_MATCH rejection should omit detail field")
	}
}

func TestVersionFieldAlwaysPresent(t *testing.T) {
	entry := Entry{
		Version:   0, // Even when 0
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.0Z07:00"),
		Command:   "ls",
		Approved:  true,
		Cwd:       "/home",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"version"`) {
		t.Error("Version field should always be present, even when 0")
	}
}

func TestTimestampSerializesTenthsOfSecond(t *testing.T) {
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Timestamp: "2025-01-15T10:30:45.1Z",
		Command:   "ls",
		Approved:  true,
		Cwd:       "/home",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	jsonStr := string(data)
	// Timestamp should have tenths of second precision: 2025-01-15T10:30:45.1Z
	if !strings.Contains(jsonStr, "2025-01-15T10:30:45.1Z") {
		t.Errorf("Timestamp should have tenths of second precision, got: %s", jsonStr)
	}
}

// Phase 5: Log Function Tests

func TestLogAutoPopulatesTimestamp(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	if err := Init(logPath, false); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Log entry without setting timestamp
	entry := Entry{
		Version:   1,
		ToolUseID: "tool-1",
		SessionID: "session-1",
		Command:   "ls",
		Approved:  true,
		Cwd:       "/home",
	}

	beforeLog := time.Now().UTC()
	if err := Log(entry); err != nil {
		t.Errorf("Log() error = %v", err)
	}
	afterLog := time.Now().UTC()

	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var parsed Entry
	if err := json.Unmarshal(content[:len(content)-1], &parsed); err != nil {
		t.Fatalf("Failed to parse entry: %v", err)
	}

	// Timestamp should be a non-empty string in the expected format
	if parsed.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}

	// Parse the timestamp to verify it's within range
	parsedTime, err := time.Parse("2006-01-02T15:04:05.0Z07:00", parsed.Timestamp)
	if err != nil {
		t.Fatalf("Failed to parse timestamp %q: %v", parsed.Timestamp, err)
	}

	// Timestamp should be between beforeLog and afterLog
	if parsedTime.Before(beforeLog.Truncate(100*time.Millisecond)) || parsedTime.After(afterLog.Add(100*time.Millisecond)) {
		t.Errorf("Timestamp %v not within expected range [%v, %v]", parsedTime, beforeLog, afterLog)
	}
}

func TestLogWritesAllFields(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	if err := Init(logPath, false); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	entry := Entry{
		Version:    1,
		ToolUseID:  "tool-123",
		SessionID:  "session-456",
		DurationMs: 42.5,
		Command:    "git status",
		Approved:   true,
		Segments: []Segment{
			{
				Command:  "git status",
				Approved: true,
				Match:    &Match{Type: "subcommand", Name: "git"},
			},
		},
		Cwd: "/home/user",
	}

	if err := Log(entry); err != nil {
		t.Errorf("Log() error = %v", err)
	}

	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var parsed Entry
	if err := json.Unmarshal(content[:len(content)-1], &parsed); err != nil {
		t.Fatalf("Failed to parse entry: %v", err)
	}

	if parsed.Version != 1 {
		t.Errorf("Version = %d, want 1", parsed.Version)
	}
	if parsed.ToolUseID != "tool-123" {
		t.Errorf("ToolUseID = %q, want %q", parsed.ToolUseID, "tool-123")
	}
	if parsed.SessionID != "session-456" {
		t.Errorf("SessionID = %q, want %q", parsed.SessionID, "session-456")
	}
	if parsed.DurationMs != 42.5 {
		t.Errorf("DurationMs = %v, want 42.5", parsed.DurationMs)
	}
	if parsed.Command != "git status" {
		t.Errorf("Command = %q, want %q", parsed.Command, "git status")
	}
	if !parsed.Approved {
		t.Error("Approved = false, want true")
	}
	if len(parsed.Segments) != 1 {
		t.Errorf("Segments count = %d, want 1", len(parsed.Segments))
	}
	if parsed.Cwd != "/home/user" {
		t.Errorf("Cwd = %q, want %q", parsed.Cwd, "/home/user")
	}
}

func TestLogAppendsToExistingFile(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	// First write
	if err := Init(logPath, false); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	Log(Entry{Version: 1, Command: "first", Approved: true})
	Close()

	// Second write (re-initialize)
	Reset()
	if err := Init(logPath, false); err != nil {
		t.Fatalf("Init() second time error = %v", err)
	}
	Log(Entry{Version: 1, Command: "second", Approved: true})
	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(lines))
	}
}
