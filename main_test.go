package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dgerlanc/mmi/patterns"
)

// TestMain sets up config for all tests
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "mmi-test-*")
	if err != nil {
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	initConfig()
	os.Exit(m.Run())
}

// =============================================================================
// Core Logic Tests
// =============================================================================

func TestMainFunction(t *testing.T) {
	origStdin := os.Stdin
	origExit := osExit
	defer func() {
		os.Stdin = origStdin
		osExit = origExit
	}()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	go func() {
		w.WriteString(`{"tool_name":"Bash","tool_input":{"command":"git status"}}`)
		w.Close()
	}()

	os.Stdin = r

	var exitCode int
	osExit = func(code int) { exitCode = code }

	main()

	if exitCode != 0 {
		t.Errorf("main() exitCode = %d, want 0", exitCode)
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectOutput bool
	}{
		{"safe command", `{"tool_name":"Bash","tool_input":{"command":"git status"}}`, true},
		{"unsafe command", `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`, false},
		{"non-Bash tool", `{"tool_name":"Read","tool_input":{"file":"/etc/passwd"}}`, false},
		{"invalid JSON", "invalid json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			exitCode := run(strings.NewReader(tt.input), &stdout)

			if exitCode != 0 {
				t.Errorf("run() exitCode = %d, want 0", exitCode)
			}

			if tt.expectOutput {
				if stdout.Len() == 0 {
					t.Error("run() expected output, got none")
				}
				var output HookOutput
				if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &output); err != nil {
					t.Errorf("run() output is not valid JSON: %v", err)
				}
			} else {
				if stdout.Len() != 0 {
					t.Errorf("run() expected no output, got %q", stdout.String())
				}
			}
		})
	}
}

func TestProcess(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectApproved bool
		expectReason   string
	}{
		// Safe commands
		{"git subcommand", `{"tool_name":"Bash","tool_input":{"command":"git status"}}`, true, "git"},
		{"simple command", `{"tool_name":"Bash","tool_input":{"command":"pytest"}}`, true, "simple"},
		{"chained commands", `{"tool_name":"Bash","tool_input":{"command":"git add . && git status"}}`, true, "git | git"},
		{"with wrapper", `{"tool_name":"Bash","tool_input":{"command":"timeout 30 pytest -v"}}`, true, "timeout + simple"},
		{"env vars wrapper", `{"tool_name":"Bash","tool_input":{"command":"FOO=bar pytest"}}`, true, "env vars + simple"},
		{"venv wrapper", `{"tool_name":"Bash","tool_input":{"command":".venv/bin/python script.py"}}`, true, ".venv + simple"},

		// Unsafe commands
		{"command substitution", `{"tool_name":"Bash","tool_input":{"command":"echo $(whoami)"}}`, false, ""},
		{"backticks", "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"echo `whoami`\"}}", false, ""},
		{"unsafe command", `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`, false, ""},
		{"unsafe in chain", `{"tool_name":"Bash","tool_input":{"command":"git status && rm -rf /"}}`, false, ""},

		// Edge cases
		{"non-Bash tool", `{"tool_name":"Read","tool_input":{"file":"/etc/passwd"}}`, false, ""},
		{"invalid JSON", "invalid json {{{", false, ""},
		{"empty command", `{"tool_name":"Bash","tool_input":{"command":""}}`, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approved, reason := process(strings.NewReader(tt.input))
			if approved != tt.expectApproved {
				t.Errorf("process() approved = %v, want %v", approved, tt.expectApproved)
			}
			if reason != tt.expectReason {
				t.Errorf("process() reason = %q, want %q", reason, tt.expectReason)
			}
		})
	}
}

func TestFormatApproval(t *testing.T) {
	result := formatApproval("test reason")

	if !strings.HasSuffix(result, "\n") {
		t.Error("formatApproval() should end with newline")
	}

	var output HookOutput
	if err := json.Unmarshal([]byte(strings.TrimSuffix(result, "\n")), &output); err != nil {
		t.Errorf("formatApproval() returned invalid JSON: %v", err)
		return
	}

	if output.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("HookEventName = %q, want %q", output.HookSpecificOutput.HookEventName, "PreToolUse")
	}
	if output.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("PermissionDecision = %q, want %q", output.HookSpecificOutput.PermissionDecision, "allow")
	}
}

func TestSplitCommandChain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"simple", "ls -la", []string{"ls -la"}},
		{"empty", "", nil},
		{"AND chain", "cmd1 && cmd2", []string{"cmd1", "cmd2"}},
		{"OR chain", "cmd1 || cmd2", []string{"cmd1", "cmd2"}},
		{"pipe", "cmd1 | cmd2", []string{"cmd1", "cmd2"}},
		{"quoted AND", `echo "a && b"`, []string{`echo "a && b"`}},
		// Shell parser correctly separates redirections from commands
		{"redirection", "cmd 2>&1", []string{"cmd"}},
		// Shell parser normalizes backslash-newline continuations
		{"backslash newline", "cmd \\\n arg", []string{"cmd \\\n\targ"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCommandChain(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("splitCommandChain(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStripWrappers(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedCore     string
		expectedWrappers []string
	}{
		{"no wrapper", "pytest", "pytest", nil},
		{"timeout", "timeout 30 pytest", "pytest", []string{"timeout"}},
		{"nice -n", "nice -n 10 pytest", "pytest", []string{"nice"}},
		{"nice -n compact", "nice -n10 pytest", "pytest", []string{"nice"}},
		{"env", "env pytest", "pytest", []string{"env"}},
		{"env vars", "FOO=bar pytest", "pytest", []string{"env vars"}},
		{".venv/bin/", ".venv/bin/python", "python", []string{".venv"}},
		{"absolute venv", "/home/user/.venv/bin/python", "python", []string{".venv"}},
		{"do", "do echo test", "echo test", []string{"do"}},
		{"chained", "timeout 30 nice pytest", "pytest", []string{"timeout", "nice"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCore, gotWrappers := stripWrappers(tt.input)
			if gotCore != tt.expectedCore {
				t.Errorf("stripWrappers(%q) core = %q, want %q", tt.input, gotCore, tt.expectedCore)
			}
			if !reflect.DeepEqual(gotWrappers, tt.expectedWrappers) {
				t.Errorf("stripWrappers(%q) wrappers = %v, want %v", tt.input, gotWrappers, tt.expectedWrappers)
			}
		})
	}
}

// =============================================================================
// checkSafe() Tests - One representative per config section type
// =============================================================================

func TestCheckSafe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Subcommand patterns (commands with specific subcommands)
		{"git subcommand", "git status", "git"},
		{"git with flag", "git -C /path diff", "git"},
		{"npm subcommand", "npm install", "npm"},
		{"cargo subcommand", "cargo build --release", "cargo"},
		{"uv subcommand", "uv pip install pkg", "uv"},
		{"maturin subcommand", "maturin develop", "maturin"},

		// Simple patterns (any args allowed)
		{"simple command", "pytest -v tests/", "simple"},
		{"read-only command", "ls -la", "read-only"},
		{"process-mgmt", "kill 1234", "process-mgmt"},
		{"loops", "done", "loops"},

		// Regex patterns
		{"shell builtin", "true", "shell builtin"},
		{"cd", "cd /path", "cd"},
		{"venv activate", "source .venv/bin/activate", "venv activate"},
		{"var assignment", "FOO=bar", "var assignment"},
		{"for loop", "for x in a b c", "for loop"},
		{"while loop", "while true", "while loop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkSafe(tt.input)
			if got != tt.expected {
				t.Errorf("checkSafe(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCheckSafeUnsafe(t *testing.T) {
	unsafeCommands := []string{
		"rm -rf /",
		"sudo anything",
		"eval code",
		"chmod 777 file",
		"wget url",
		"pip install pkg",
		"unknowncommand",
		"",
	}

	for _, cmd := range unsafeCommands {
		t.Run(cmd, func(t *testing.T) {
			if got := checkSafe(cmd); got != "" {
				t.Errorf("checkSafe(%q) = %q, want empty (unsafe)", cmd, got)
			}
		})
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func runMmi(t *testing.T, input string) (string, int) {
	t.Helper()

	cmd := exec.Command("go", "build", "-o", "mmi_test_binary", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build: %v", err)
	}
	defer os.Remove("mmi_test_binary")

	cmd = exec.Command("./mmi_test_binary")
	cmd.Stdin = strings.NewReader(input)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}

	return stdout.String(), exitCode
}

func TestIntegration(t *testing.T) {
	t.Run("safe command produces output", func(t *testing.T) {
		input := `{"tool_name":"Bash","tool_input":{"command":"git status"}}`
		output, exitCode := runMmi(t, input)

		if exitCode != 0 {
			t.Errorf("Expected exit 0, got %d", exitCode)
		}
		if output == "" {
			t.Error("Expected approval output")
		}

		var result HookOutput
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Errorf("Failed to parse output: %v", err)
		}
		if result.HookSpecificOutput.PermissionDecision != "allow" {
			t.Errorf("Expected 'allow', got %q", result.HookSpecificOutput.PermissionDecision)
		}
	})

	t.Run("unsafe command produces no output", func(t *testing.T) {
		input := `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`
		output, _ := runMmi(t, input)

		if output != "" {
			t.Errorf("Expected no output, got %q", output)
		}
	})

	t.Run("non-Bash tool produces no output", func(t *testing.T) {
		input := `{"tool_name":"Read","tool_input":{"file":"/etc/passwd"}}`
		output, _ := runMmi(t, input)

		if output != "" {
			t.Errorf("Expected no output, got %q", output)
		}
	})

	t.Run("invalid JSON produces no output", func(t *testing.T) {
		output, exitCode := runMmi(t, "invalid json")

		if output != "" {
			t.Errorf("Expected no output, got %q", output)
		}
		if exitCode != 0 {
			t.Errorf("Expected exit 0, got %d", exitCode)
		}
	})
}

func TestDangerousPatterns(t *testing.T) {
	dangerous := []string{
		`echo "$(whoami)"`,
		"echo '$(whoami)'",
		"echo \"`whoami`\"",
	}

	for _, cmd := range dangerous {
		if !dangerousPattern.MatchString(cmd) {
			t.Errorf("Expected dangerous pattern to match %q", cmd)
		}
	}
}

// =============================================================================
// Config Tests
// =============================================================================

func TestGetConfigDir(t *testing.T) {
	t.Run("with MMI_CONFIG", func(t *testing.T) {
		origVal := os.Getenv("MMI_CONFIG")
		defer os.Setenv("MMI_CONFIG", origVal)

		os.Setenv("MMI_CONFIG", "/custom/path")
		dir, err := getConfigDir()
		if err != nil {
			t.Errorf("getConfigDir() error = %v", err)
		}
		if dir != "/custom/path" {
			t.Errorf("getConfigDir() = %q, want /custom/path", dir)
		}
	})

	t.Run("without MMI_CONFIG", func(t *testing.T) {
		origVal := os.Getenv("MMI_CONFIG")
		defer os.Setenv("MMI_CONFIG", origVal)

		os.Unsetenv("MMI_CONFIG")
		dir, err := getConfigDir()
		if err != nil {
			t.Errorf("getConfigDir() error = %v", err)
		}
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".config", "mmi")
		if dir != expected {
			t.Errorf("getConfigDir() = %q, want %q", dir, expected)
		}
	})
}

func TestEnsureConfigFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mmi-ensure-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := filepath.Join(tmpDir, "config")
	if err := ensureConfigFiles(configDir); err != nil {
		t.Errorf("ensureConfigFiles() error = %v", err)
	}

	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.toml was not created")
	}

	// Second call should not overwrite
	originalContent, _ := os.ReadFile(configPath)
	ensureConfigFiles(configDir)
	newContent, _ := os.ReadFile(configPath)
	if !bytes.Equal(originalContent, newContent) {
		t.Error("ensureConfigFiles() overwrote existing file")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("simple commands", func(t *testing.T) {
		tomlData := []byte(`
[[commands.simple]]
name = "test"
commands = ["pytest", "python"]
`)
		_, commands, err := loadConfig(tomlData)
		if err != nil {
			t.Errorf("loadConfig() error = %v", err)
		}
		if len(commands) != 2 {
			t.Errorf("loadConfig() returned %d commands, want 2", len(commands))
		}
	})

	t.Run("subcommands with flags", func(t *testing.T) {
		tomlData := []byte(`
[[commands.subcommand]]
command = "git"
subcommands = ["diff", "log"]
flags = ["-C <arg>"]
`)
		_, commands, err := loadConfig(tomlData)
		if err != nil {
			t.Errorf("loadConfig() error = %v", err)
		}
		if len(commands) != 1 {
			t.Errorf("loadConfig() returned %d commands, want 1", len(commands))
		}
	})

	t.Run("regex patterns", func(t *testing.T) {
		tomlData := []byte(`
[[commands.regex]]
pattern = "^test\\b"
name = "test"
`)
		_, commands, err := loadConfig(tomlData)
		if err != nil {
			t.Errorf("loadConfig() error = %v", err)
		}
		if len(commands) != 1 {
			t.Errorf("loadConfig() returned %d commands, want 1", len(commands))
		}
	})

	t.Run("wrappers", func(t *testing.T) {
		tomlData := []byte(`
[[wrappers.command]]
command = "timeout"
flags = ["<arg>"]

[[wrappers.simple]]
name = "env"
commands = ["env"]
`)
		wrappers, _, err := loadConfig(tomlData)
		if err != nil {
			t.Errorf("loadConfig() error = %v", err)
		}
		if len(wrappers) != 2 {
			t.Errorf("loadConfig() returned %d wrappers, want 2", len(wrappers))
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		tomlData := []byte(`
[[commands.regex]]
pattern = "[invalid"
name = "bad"
`)
		if _, _, err := loadConfig(tomlData); err == nil {
			t.Error("loadConfig() should return error for invalid regex")
		}
	})

	t.Run("invalid TOML", func(t *testing.T) {
		if _, _, err := loadConfig([]byte(`not valid {{{`)); err == nil {
			t.Error("loadConfig() should return error for invalid TOML")
		}
	})
}

func TestInitConfig(t *testing.T) {
	origWrappers := wrapperPatterns
	origCommands := safeCommands
	origInitialized := configInitialized
	defer func() {
		wrapperPatterns = origWrappers
		safeCommands = origCommands
		configInitialized = origInitialized
	}()

	tmpDir, err := os.MkdirTemp("", "mmi-init-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origEnv := os.Getenv("MMI_CONFIG")
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Setenv("MMI_CONFIG", origEnv)

	configInitialized = false
	wrapperPatterns = nil
	safeCommands = nil

	if err := initConfig(); err != nil {
		t.Errorf("initConfig() error = %v", err)
	}

	if len(wrapperPatterns) == 0 {
		t.Error("wrapperPatterns is empty")
	}
	if len(safeCommands) == 0 {
		t.Error("safeCommands is empty")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "config.toml")); os.IsNotExist(err) {
		t.Error("config.toml was not created")
	}
}

func TestConfigCustomization(t *testing.T) {
	origWrappers := wrapperPatterns
	origCommands := safeCommands
	origInitialized := configInitialized
	defer func() {
		wrapperPatterns = origWrappers
		safeCommands = origCommands
		configInitialized = origInitialized
	}()

	tmpDir, err := os.MkdirTemp("", "mmi-custom-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configToml := []byte(`
[[wrappers.regex]]
pattern = "^custom\\s+"
name = "custom"

[[commands.subcommand]]
command = "mycommand"
subcommands = ["arg"]
`)
	os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

	origEnv := os.Getenv("MMI_CONFIG")
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Setenv("MMI_CONFIG", origEnv)

	configInitialized = false
	wrapperPatterns = nil
	safeCommands = nil

	if err := initConfig(); err != nil {
		t.Errorf("initConfig() error = %v", err)
	}

	// Verify custom patterns work
	core, wrappers := stripWrappers("custom mycommand arg")
	if len(wrappers) != 1 || wrappers[0] != "custom" {
		t.Errorf("Custom wrapper not stripped: %v", wrappers)
	}
	if checkSafe(core) != "mycommand" {
		t.Errorf("Custom command not recognized: %q", core)
	}
}

// =============================================================================
// Pattern Building Tests
// =============================================================================

func TestBuildFlagPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"<arg>", `(\S+\s+)?`},
		{"-f", `(-f\s+)?`},
		{"-f <arg>", `(-f\s*\S+\s+)?`},
		{"-C <arg>", `(-C\s*\S+\s+)?`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := patterns.BuildFlagPattern(tt.input); got != tt.expected {
				t.Errorf("BuildFlagPattern(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildSimplePattern(t *testing.T) {
	if got := patterns.BuildSimplePattern("pytest"); got != `^pytest\b` {
		t.Errorf("BuildSimplePattern(pytest) = %q, want ^pytest\\b", got)
	}
}

func TestBuildSubcommandPattern(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		subcommands []string
		flags       []string
		expected    string
	}{
		{"simple", "git", []string{"diff", "log"}, nil, `^git\s+(diff|log)\b`},
		{"with flag", "git", []string{"diff"}, []string{"-C <arg>"}, `^git\s+(-C\s*\S+\s+)?(diff)\b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := patterns.BuildSubcommandPattern(tt.cmd, tt.subcommands, tt.flags); got != tt.expected {
				t.Errorf("BuildSubcommandPattern() = %q, want %q", got, tt.expected)
			}
		})
	}
}
