// Package audit provides audit logging for mmi command approval decisions.
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgerlanc/mmi/internal/logger"
)

// Entry represents a single audit log entry.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Command   string    `json:"command"`
	Approved  bool      `json:"approved"`
	Reason    string    `json:"reason,omitempty"`
	Profile   string    `json:"profile,omitempty"`
}

var (
	auditFile *os.File
	mu        sync.Mutex
	enabled   bool
)

// DefaultLogPath returns the default audit log path (~/.local/share/mmi/audit.log)
func DefaultLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "mmi", "audit.log"), nil
}

// Init initializes the audit log. If path is empty, uses the default path.
// If path is "-" or audit logging should be disabled, pass disable=true.
func Init(path string, disable bool) error {
	mu.Lock()
	defer mu.Unlock()

	if disable {
		enabled = false
		return nil
	}

	if path == "" {
		var err error
		path, err = DefaultLogPath()
		if err != nil {
			logger.Debug("failed to get default audit log path", "error", err)
			return err
		}
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Debug("failed to create audit log directory", "error", err)
		return err
	}

	// Open audit log file in append mode
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Debug("failed to open audit log file", "error", err)
		return err
	}

	auditFile = f
	enabled = true
	logger.Debug("audit logging initialized", "path", path)
	return nil
}

// Close closes the audit log file.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if auditFile != nil {
		err := auditFile.Close()
		auditFile = nil
		enabled = false
		return err
	}
	return nil
}

// Log writes an entry to the audit log.
// If audit logging is not initialized or disabled, this is a no-op.
func Log(entry Entry) error {
	mu.Lock()
	defer mu.Unlock()

	if !enabled || auditFile == nil {
		return nil
	}

	entry.Timestamp = time.Now().UTC()

	data, err := json.Marshal(entry)
	if err != nil {
		logger.Debug("failed to marshal audit entry", "error", err)
		return err
	}

	if _, err := auditFile.Write(append(data, '\n')); err != nil {
		logger.Debug("failed to write audit entry", "error", err)
		return err
	}

	return nil
}

// IsEnabled returns whether audit logging is enabled.
func IsEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// Reset resets the audit state. Used for testing.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	if auditFile != nil {
		auditFile.Close()
	}
	auditFile = nil
	enabled = false
}
