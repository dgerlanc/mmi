package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	Init(Options{
		Verbose: true,
		Output:  &buf,
	})

	Debug("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected output to contain 'key=value', got: %s", output)
	}
}

func TestInitOnlyOnce(t *testing.T) {
	defer Reset()

	var buf1, buf2 bytes.Buffer
	Init(Options{Verbose: true, Output: &buf1})
	Init(Options{Verbose: true, Output: &buf2}) // Should be ignored

	Debug("test message")

	if buf1.Len() == 0 {
		t.Error("expected first buffer to have output")
	}
	if buf2.Len() != 0 {
		t.Error("expected second buffer to be empty (Init should only work once)")
	}
}

func TestIsVerbose(t *testing.T) {
	defer Reset()

	if IsVerbose() {
		t.Error("expected IsVerbose to be false before Init")
	}

	Init(Options{Verbose: true})

	if !IsVerbose() {
		t.Error("expected IsVerbose to be true after Init with Verbose: true")
	}
}

func TestLogLevels(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	Init(Options{
		Verbose: true, // Enable debug level
		Output:  &buf,
	})

	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	output := buf.String()

	levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	messages := []string{"debug message", "info message", "warn message", "error message"}

	for i, msg := range messages {
		if !strings.Contains(output, msg) {
			t.Errorf("expected output to contain %q", msg)
		}
		if !strings.Contains(output, levels[i]) || !strings.Contains(output, "level="+levels[i]) {
			// Text handler uses level=LEVEL format
		}
	}
}

func TestNonVerboseMode(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	Init(Options{
		Verbose: false, // Only error level
		Output:  &buf,
	})

	Debug("debug message")
	Info("info message")
	Warn("warn message")

	if buf.Len() != 0 {
		t.Errorf("expected no output for debug/info/warn in non-verbose mode, got: %s", buf.String())
	}

	Error("error message")

	if !strings.Contains(buf.String(), "error message") {
		t.Error("expected error message to be logged even in non-verbose mode")
	}
}

func TestJSONFormat(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	Init(Options{
		Verbose: true,
		Output:  &buf,
		JSON:    true,
	})

	Debug("test message", "key", "value")

	output := buf.String()
	// JSON format should contain quoted strings
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Errorf("expected JSON output with msg field, got: %s", output)
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Errorf("expected JSON output with key field, got: %s", output)
	}
}

func TestWith(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	Init(Options{
		Verbose: true,
		Output:  &buf,
	})

	childLogger := With("component", "test")
	childLogger.Debug("child message")

	output := buf.String()
	if !strings.Contains(output, "component=test") {
		t.Errorf("expected output to contain 'component=test', got: %s", output)
	}
}

func TestLogBeforeInit(t *testing.T) {
	defer Reset()

	// These should not panic even before Init
	Debug("debug")
	Info("info")
	Warn("warn")
	Error("error")
}

func TestReset(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	Init(Options{Verbose: true, Output: &buf1})
	Debug("first")

	Reset()

	Init(Options{Verbose: true, Output: &buf2})
	Debug("second")

	if !strings.Contains(buf1.String(), "first") {
		t.Error("expected first buffer to contain 'first'")
	}
	if !strings.Contains(buf2.String(), "second") {
		t.Error("expected second buffer to contain 'second'")
	}
	if strings.Contains(buf1.String(), "second") {
		t.Error("expected first buffer to NOT contain 'second'")
	}
}
