// Package logger provides structured logging for mmi using log/slog.
package logger

import (
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	log     *slog.Logger
	once    sync.Once
	verbose bool
)

// Options configures the logger.
type Options struct {
	// Verbose enables debug-level logging
	Verbose bool
	// Output is the writer for log output (defaults to os.Stderr)
	Output io.Writer
	// JSON enables JSON-formatted output
	JSON bool
}

// Init initializes the global logger with the given options.
// It is safe to call multiple times; only the first call takes effect.
func Init(opts Options) {
	once.Do(func() {
		verbose = opts.Verbose

		output := opts.Output
		if output == nil {
			output = os.Stderr
		}

		level := slog.LevelError
		if opts.Verbose {
			level = slog.LevelDebug
		}

		handlerOpts := &slog.HandlerOptions{Level: level}

		var handler slog.Handler
		if opts.JSON {
			handler = slog.NewJSONHandler(output, handlerOpts)
		} else {
			handler = slog.NewTextHandler(output, handlerOpts)
		}

		log = slog.New(handler)
	})
}

// Reset resets the logger for testing purposes.
// This should only be used in tests.
func Reset() {
	once = sync.Once{}
	log = nil
	verbose = false
}

// IsVerbose returns true if verbose logging is enabled.
func IsVerbose() bool {
	return verbose
}

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	if log != nil {
		log.Debug(msg, args...)
	}
}

// Info logs at info level.
func Info(msg string, args ...any) {
	if log != nil {
		log.Info(msg, args...)
	}
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	if log != nil {
		log.Warn(msg, args...)
	}
}

// Error logs at error level.
func Error(msg string, args ...any) {
	if log != nil {
		log.Error(msg, args...)
	}
}

// With returns a logger with additional context attributes.
func With(args ...any) *slog.Logger {
	if log == nil {
		return slog.Default()
	}
	return log.With(args...)
}
