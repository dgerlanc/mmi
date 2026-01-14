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

	if err := Init(logPath, false, nil); err != nil {
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

	if err := Init("", true, nil); err != nil {
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

	if err := Init(logPath, false, nil); err != nil {
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

func TestLogWithProfile(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	if err := Init(logPath, false, nil); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	entry := Entry{
		Command:  "git status",
		Approved: true,
		Reason:   "git",
		Profile:  "minimal",
	}
	if err := Log(entry); err != nil {
		t.Errorf("Log() error = %v", err)
	}

	Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), `"profile":"minimal"`) {
		t.Error("Log entry should contain profile")
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

	if err := Init(logPath, false, nil); err != nil {
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

func TestLogRotation(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	// Initialize with small max size to trigger rotation
	cfg := &CompactionConfig{
		MaxSize:    100, // 100 bytes - very small to trigger rotation easily
		MaxBackups: 3,
		Compress:   true,
	}

	if err := Init(logPath, false, cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer Close()

	// Log enough entries to trigger rotation
	for i := 0; i < 10; i++ {
		entry := Entry{
			Command:  "test command with some text to make it longer",
			Approved: true,
			Reason:   "test",
		}
		if err := Log(entry); err != nil {
			t.Errorf("Log() error = %v", err)
		}
	}

	// Check that rotated files exist
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	var logFiles []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "audit.log") {
			logFiles = append(logFiles, entry.Name())
		}
	}

	// Should have audit.log plus some rotated files
	if len(logFiles) < 2 {
		t.Errorf("Expected at least 2 log files (current + rotated), got %d: %v", len(logFiles), logFiles)
	}

	// Check for compressed files
	hasCompressed := false
	for _, name := range logFiles {
		if strings.HasSuffix(name, ".gz") {
			hasCompressed = true
			break
		}
	}
	if !hasCompressed {
		t.Error("Expected at least one compressed log file")
	}
}

func TestLogRotationMaxBackups(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	// Initialize with very small max size and max backups
	cfg := &CompactionConfig{
		MaxSize:    50, // Very small to trigger multiple rotations
		MaxBackups: 2,  // Keep only 2 old files
		Compress:   false,
	}

	if err := Init(logPath, false, cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer Close()

	// Log many entries to trigger multiple rotations
	for i := 0; i < 20; i++ {
		entry := Entry{
			Command:  "test command with text",
			Approved: true,
		}
		if err := Log(entry); err != nil {
			t.Errorf("Log() error = %v", err)
		}
	}

	// Count backup files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	backupCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if name != "audit.log" && strings.HasPrefix(name, "audit.log.") {
			backupCount++
		}
	}

	// Should have at most MaxBackups backup files
	if backupCount > cfg.MaxBackups {
		t.Errorf("Expected at most %d backup files, got %d", cfg.MaxBackups, backupCount)
	}
}

func TestNoRotationWhenDisabled(t *testing.T) {
	defer Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "audit.log")

	// Initialize with MaxSize = 0 (rotation disabled)
	cfg := &CompactionConfig{
		MaxSize:    0,
		MaxBackups: 5,
		Compress:   true,
	}

	if err := Init(logPath, false, cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer Close()

	// Log many entries
	for i := 0; i < 100; i++ {
		entry := Entry{
			Command:  "test command with some text to make the file large",
			Approved: true,
		}
		if err := Log(entry); err != nil {
			t.Errorf("Log() error = %v", err)
		}
	}

	// Should only have the main log file, no rotations
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	logFileCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "audit.log") {
			logFileCount++
		}
	}

	if logFileCount != 1 {
		t.Errorf("Expected exactly 1 log file when rotation disabled, got %d", logFileCount)
	}
}
