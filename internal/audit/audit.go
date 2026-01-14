// Package audit provides audit logging for mmi command approval decisions.
package audit

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// CompactionConfig holds configuration for log rotation and compression.
type CompactionConfig struct {
	// MaxSize is the maximum size in bytes before rotation (0 = no rotation)
	MaxSize int64
	// MaxBackups is the maximum number of old log files to retain (0 = keep all)
	MaxBackups int
	// Compress indicates whether to gzip rotated log files
	Compress bool
}

// DefaultCompactionConfig returns sensible defaults for log compaction.
// MaxSize: 10MB, MaxBackups: 5, Compress: true
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		MaxSize:    10 * 1024 * 1024, // 10MB
		MaxBackups: 5,
		Compress:   true,
	}
}

var (
	auditFile     *os.File
	auditPath     string
	mu            sync.Mutex
	enabled       bool
	compactionCfg CompactionConfig
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
// Pass nil for cfg to use default compaction settings, or a zero-value config to disable compaction.
func Init(path string, disable bool, cfg *CompactionConfig) error {
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

	// Set compaction config (use defaults if nil)
	if cfg == nil {
		compactionCfg = DefaultCompactionConfig()
	} else {
		compactionCfg = *cfg
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
	auditPath = path
	enabled = true
	logger.Debug("audit logging initialized",
		"path", path,
		"maxSize", compactionCfg.MaxSize,
		"maxBackups", compactionCfg.MaxBackups,
		"compress", compactionCfg.Compress)
	return nil
}

// Close closes the audit log file.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if auditFile != nil {
		err := auditFile.Close()
		auditFile = nil
		auditPath = ""
		enabled = false
		return err
	}
	return nil
}

// Log writes an entry to the audit log.
// If audit logging is not initialized or disabled, this is a no-op.
// Automatically rotates the log file if it exceeds the configured max size.
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

	// Check if rotation is needed (if maxSize is configured)
	if compactionCfg.MaxSize > 0 {
		if err := checkAndRotate(); err != nil {
			logger.Debug("failed to rotate audit log", "error", err)
			// Don't fail the log write if rotation fails
		}
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
	auditPath = ""
	enabled = false
	compactionCfg = CompactionConfig{}
}

// checkAndRotate checks if the current log file exceeds maxSize and rotates if needed.
// Must be called with mu locked.
func checkAndRotate() error {
	// Get current file size
	info, err := auditFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat audit log: %w", err)
	}

	// Check if rotation is needed
	if info.Size() < compactionCfg.MaxSize {
		return nil
	}

	logger.Debug("rotating audit log", "size", info.Size(), "maxSize", compactionCfg.MaxSize)

	// Close current file
	if err := auditFile.Close(); err != nil {
		return fmt.Errorf("failed to close audit log for rotation: %w", err)
	}

	// Perform rotation
	if err := rotateFiles(); err != nil {
		// Try to reopen the file even if rotation failed
		f, reopenErr := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if reopenErr != nil {
			return fmt.Errorf("rotation failed and could not reopen file: %v (original error: %w)", reopenErr, err)
		}
		auditFile = f
		return fmt.Errorf("rotation failed: %w", err)
	}

	// Open new file
	f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to reopen audit log after rotation: %w", err)
	}

	auditFile = f
	logger.Debug("audit log rotated successfully")
	return nil
}

// rotateFiles performs the actual file rotation.
// Renames audit.log -> audit.log.1, audit.log.1 -> audit.log.2, etc.
// Compresses and cleans up old files based on configuration.
func rotateFiles() error {
	// Get list of existing backup files
	backups, err := getBackupFiles()
	if err != nil {
		return fmt.Errorf("failed to get backup files: %w", err)
	}

	// Delete excess backups
	if compactionCfg.MaxBackups > 0 && len(backups) >= compactionCfg.MaxBackups {
		// Sort by number (oldest first)
		sort.Slice(backups, func(i, j int) bool {
			return backups[i].num < backups[j].num
		})

		// Delete oldest files beyond the limit
		for i := 0; i <= len(backups)-compactionCfg.MaxBackups; i++ {
			if err := os.Remove(backups[i].path); err != nil && !os.IsNotExist(err) {
				logger.Debug("failed to remove old backup", "path", backups[i].path, "error", err)
			} else {
				logger.Debug("removed old backup", "path", backups[i].path)
			}
		}

		// Remove deleted files from the list
		backups = backups[len(backups)-compactionCfg.MaxBackups+1:]
	}

	// Rotate existing backups (audit.log.1 -> audit.log.2, etc.)
	// Process in reverse order to avoid overwriting
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].num > backups[j].num
	})

	for _, backup := range backups {
		newPath := fmt.Sprintf("%s.%d", auditPath, backup.num+1)
		if backup.compressed {
			newPath += ".gz"
		}
		if err := os.Rename(backup.path, newPath); err != nil {
			return fmt.Errorf("failed to rotate %s to %s: %w", backup.path, newPath, err)
		}
		logger.Debug("rotated backup", "from", backup.path, "to", newPath)
	}

	// Rotate current log file to .1
	newPath := fmt.Sprintf("%s.1", auditPath)
	if err := os.Rename(auditPath, newPath); err != nil {
		return fmt.Errorf("failed to rotate current log: %w", err)
	}
	logger.Debug("rotated current log", "to", newPath)

	// Compress the newly rotated file if configured
	if compactionCfg.Compress {
		if err := compressFile(newPath); err != nil {
			logger.Debug("failed to compress rotated log", "path", newPath, "error", err)
			// Don't fail rotation if compression fails
		}
	}

	return nil
}

// backupFile represents a backup log file
type backupFile struct {
	path       string
	num        int
	compressed bool
}

// getBackupFiles returns a list of existing backup files
func getBackupFiles() ([]backupFile, error) {
	dir := filepath.Dir(auditPath)
	base := filepath.Base(auditPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var backups []backupFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		var num int
		var compressed bool

		// Check for .gz extension
		if filepath.Ext(name) == ".gz" {
			compressed = true
			name = name[:len(name)-3] // Remove .gz
		}

		// Match audit.log.N pattern
		if _, err := fmt.Sscanf(name, base+".%d", &num); err == nil {
			backups = append(backups, backupFile{
				path:       filepath.Join(dir, entry.Name()),
				num:        num,
				compressed: compressed,
			})
		}
	}

	return backups, nil
}

// compressFile compresses a file using gzip and removes the original
func compressFile(path string) error {
	// Open source file
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	// Create compressed file
	dstPath := path + ".gz"
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create compressed file: %w", err)
	}
	defer dst.Close()

	// Create gzip writer
	gzw := gzip.NewWriter(dst)
	defer gzw.Close()

	// Copy data
	if _, err := io.Copy(gzw, src); err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}

	// Close gzip writer to flush
	if err := gzw.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Close destination file
	if err := dst.Close(); err != nil {
		return fmt.Errorf("failed to close compressed file: %w", err)
	}

	// Remove original file
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove original file: %w", err)
	}

	logger.Debug("compressed file", "from", path, "to", dstPath)
	return nil
}
