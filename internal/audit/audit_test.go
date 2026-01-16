package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		Command:  "git status",
		Approved: true,
		Reason:   "git",
	}
	if err := Log(entry1); err != nil {
		t.Errorf("Log() error = %v", err)
	}

	// Log a rejected command
	entry2 := Entry{
		Command:  "rm -rf /",
		Approved: false,
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
	if parsed1.Reason != "git" {
		t.Errorf("First entry reason = %q, want %q", parsed1.Reason, "git")
	}
	if parsed1.Timestamp.IsZero() {
		t.Error("First entry timestamp should not be zero")
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
		Command:  "git status",
		Approved: true,
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
