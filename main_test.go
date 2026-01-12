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

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/hook"
	"github.com/dgerlanc/mmi/internal/patterns"
)

// testConfig contains all patterns needed for testing
const testConfig = `
# Test configuration with comprehensive patterns for testing

# Wrappers
[[wrappers.simple]]
name = "env"
commands = ["env", "do"]

[[wrappers.command]]
command = "timeout"
flags = ["<arg>"]

[[wrappers.command]]
command = "nice"
flags = ["-n <arg>", ""]

[[wrappers.regex]]
pattern = '^([A-Z_][A-Z0-9_]*=[^\s]*\s+)+'
name = "env vars"

[[wrappers.regex]]
pattern = '^([^\s]*/)?\.venv/bin/'
name = ".venv"

# Commands - subcommands
[[commands.subcommand]]
command = "git"
subcommands = ["status", "add", "diff", "log", "commit", "push", "pull", "fetch", "checkout", "branch", "merge", "rebase", "stash", "reset", "show", "tag", "clone", "remote", "init"]
flags = ["-C <arg>"]

[[commands.subcommand]]
command = "npm"
subcommands = ["install", "run", "test", "build", "start", "ci", "audit", "outdated", "list", "ls", "view", "init", "publish", "pack", "link", "unlink", "update", "prune", "dedupe", "cache"]

[[commands.subcommand]]
command = "cargo"
subcommands = ["build", "run", "test", "bench", "check", "clean", "doc", "publish", "install", "uninstall", "search", "update", "fmt", "clippy", "add", "remove", "tree", "metadata", "locate-project", "verify-project", "fetch", "generate-lockfile", "new", "init"]
flags = ["--release", "-p <arg>", "--package <arg>", "--bin <arg>", "--lib", "--features <arg>", "--all-features", "--no-default-features"]

[[commands.subcommand]]
command = "uv"
subcommands = ["pip", "run", "sync", "venv", "add", "remove", "lock"]

[[commands.subcommand]]
command = "maturin"
subcommands = ["build", "develop", "publish", "sdist", "init", "new", "list-python", "upload"]

# Commands - simple
[[commands.simple]]
name = "simple"
commands = ["pytest", "python", "python3", "node", "ruby", "perl", "php", "java", "javac", "scala", "go", "rustc", "gcc", "g++", "clang", "make", "cmake", "ninja", "ruff", "black", "isort", "mypy", "pylint", "flake8", "eslint", "prettier", "tsc"]

[[commands.simple]]
name = "read-only"
commands = ["ls", "cat", "head", "tail", "less", "more", "file", "stat", "wc", "du", "df", "pwd", "whoami", "hostname", "date", "cal", "uptime", "free", "which", "whereis", "type", "printenv", "env", "id", "groups", "w", "who", "last", "ps", "pgrep", "top", "htop", "lsof", "netstat", "ss", "ip", "ifconfig", "ping", "traceroute", "dig", "nslookup", "host", "curl", "tree", "find", "locate", "grep", "egrep", "fgrep", "rg", "ag", "ack", "sed", "awk", "cut", "sort", "uniq", "diff", "comm", "join", "paste", "tr", "tee", "xargs", "strings", "xxd", "hexdump", "od", "base64", "md5sum", "sha256sum", "sha1sum", "jq", "yq", "xmllint"]

[[commands.simple]]
name = "process-mgmt"
commands = ["kill", "pkill", "killall"]

[[commands.simple]]
name = "loops"
commands = ["done", "fi", "esac"]

# Commands - regex
[[commands.regex]]
pattern = '^(true|false|exit(\s+\d+)?)$'
name = "shell builtin"

[[commands.regex]]
pattern = '^source\s+.*\.venv.*/bin/activate'
name = "venv activate"

[[commands.regex]]
pattern = '^[A-Z_][A-Z0-9_]*=\S*$'
name = "var assignment"

[[commands.regex]]
pattern = '^for\s+\w+\s+in\s'
name = "for loop"

[[commands.regex]]
pattern = '^while\s'
name = "while loop"
`

// TestMain sets up config for all tests
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "mmi-test-*")
	if err != nil {
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Write test-specific config
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		os.Exit(1)
	}

	config.Init()
	os.Exit(m.Run())
}

// =============================================================================
// Core Logic Tests
// =============================================================================

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
			approved, reason := hook.Process(strings.NewReader(tt.input))
			if approved != tt.expectApproved {
				t.Errorf("Process() approved = %v, want %v", approved, tt.expectApproved)
			}
			if reason != tt.expectReason {
				t.Errorf("Process() reason = %q, want %q", reason, tt.expectReason)
			}
		})
	}
}

func TestFormatApproval(t *testing.T) {
	result := hook.FormatApproval("test reason")

	if !strings.HasSuffix(result, "\n") {
		t.Error("FormatApproval() should end with newline")
	}

	var output hook.Output
	if err := json.Unmarshal([]byte(strings.TrimSuffix(result, "\n")), &output); err != nil {
		t.Errorf("FormatApproval() returned invalid JSON: %v", err)
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
			got := hook.SplitCommandChain(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("SplitCommandChain(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStripWrappers(t *testing.T) {
	cfg := config.Get()

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
			gotCore, gotWrappers := hook.StripWrappers(tt.input, cfg.WrapperPatterns)
			if gotCore != tt.expectedCore {
				t.Errorf("StripWrappers(%q) core = %q, want %q", tt.input, gotCore, tt.expectedCore)
			}
			if !reflect.DeepEqual(gotWrappers, tt.expectedWrappers) {
				t.Errorf("StripWrappers(%q) wrappers = %v, want %v", tt.input, gotWrappers, tt.expectedWrappers)
			}
		})
	}
}

// =============================================================================
// CheckSafe() Tests - One representative per config section type
// =============================================================================

func TestCheckSafe(t *testing.T) {
	cfg := config.Get()

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
		{"venv activate", "source .venv/bin/activate", "venv activate"},
		{"var assignment", "FOO=bar", "var assignment"},
		{"for loop", "for x in a b c", "for loop"},
		{"while loop", "while true", "while loop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hook.CheckSafe(tt.input, cfg.SafeCommands)
			if got != tt.expected {
				t.Errorf("CheckSafe(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCheckSafeUnsafe(t *testing.T) {
	cfg := config.Get()

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
			if got := hook.CheckSafe(cmd, cfg.SafeCommands); got != "" {
				t.Errorf("CheckSafe(%q) = %q, want empty (unsafe)", cmd, got)
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
	cmd.Env = os.Environ() // Inherit environment including MMI_CONFIG
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

		var result hook.Output
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

// =============================================================================
// Config Tests
// =============================================================================

func TestGetConfigDir(t *testing.T) {
	t.Run("with MMI_CONFIG", func(t *testing.T) {
		origVal := os.Getenv("MMI_CONFIG")
		defer os.Setenv("MMI_CONFIG", origVal)

		os.Setenv("MMI_CONFIG", "/custom/path")
		dir, err := config.GetConfigDir()
		if err != nil {
			t.Errorf("GetConfigDir() error = %v", err)
		}
		if dir != "/custom/path" {
			t.Errorf("GetConfigDir() = %q, want /custom/path", dir)
		}
	})

	t.Run("without MMI_CONFIG", func(t *testing.T) {
		origVal := os.Getenv("MMI_CONFIG")
		defer os.Setenv("MMI_CONFIG", origVal)

		os.Unsetenv("MMI_CONFIG")
		dir, err := config.GetConfigDir()
		if err != nil {
			t.Errorf("GetConfigDir() error = %v", err)
		}
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".config", "mmi")
		if dir != expected {
			t.Errorf("GetConfigDir() = %q, want %q", dir, expected)
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
	if err := config.EnsureConfigFiles(configDir); err != nil {
		t.Errorf("EnsureConfigFiles() error = %v", err)
	}

	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.toml was not created")
	}

	// Second call should not overwrite
	originalContent, _ := os.ReadFile(configPath)
	config.EnsureConfigFiles(configDir)
	newContent, _ := os.ReadFile(configPath)
	if !bytes.Equal(originalContent, newContent) {
		t.Error("EnsureConfigFiles() overwrote existing file")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("simple commands", func(t *testing.T) {
		tomlData := []byte(`
[[commands.simple]]
name = "test"
commands = ["pytest", "python"]
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if len(cfg.SafeCommands) != 2 {
			t.Errorf("LoadConfig() returned %d commands, want 2", len(cfg.SafeCommands))
		}
	})

	t.Run("subcommands with flags", func(t *testing.T) {
		tomlData := []byte(`
[[commands.subcommand]]
command = "git"
subcommands = ["diff", "log"]
flags = ["-C <arg>"]
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if len(cfg.SafeCommands) != 1 {
			t.Errorf("LoadConfig() returned %d commands, want 1", len(cfg.SafeCommands))
		}
	})

	t.Run("regex patterns", func(t *testing.T) {
		tomlData := []byte(`
[[commands.regex]]
pattern = "^test\\b"
name = "test"
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if len(cfg.SafeCommands) != 1 {
			t.Errorf("LoadConfig() returned %d commands, want 1", len(cfg.SafeCommands))
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
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if len(cfg.WrapperPatterns) != 2 {
			t.Errorf("LoadConfig() returned %d wrappers, want 2", len(cfg.WrapperPatterns))
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		tomlData := []byte(`
[[commands.regex]]
pattern = "[invalid"
name = "bad"
`)
		if _, err := config.LoadConfig(tomlData); err == nil {
			t.Error("LoadConfig() should return error for invalid regex")
		}
	})

	t.Run("invalid TOML", func(t *testing.T) {
		if _, err := config.LoadConfig([]byte(`not valid {{{`)); err == nil {
			t.Error("LoadConfig() should return error for invalid TOML")
		}
	})

	t.Run("general section defaults", func(t *testing.T) {
		tomlData := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if cfg.General.AllowSubshells {
			t.Error("AllowSubshells should default to false")
		}
		if cfg.General.AllowBackticks {
			t.Error("AllowBackticks should default to false")
		}
	})

	t.Run("general section allow subshells", func(t *testing.T) {
		tomlData := []byte(`
[general]
allow_subshells = true
allow_backticks = false

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if !cfg.General.AllowSubshells {
			t.Error("AllowSubshells should be true")
		}
		if cfg.General.AllowBackticks {
			t.Error("AllowBackticks should be false")
		}
	})

	t.Run("general section allow backticks", func(t *testing.T) {
		tomlData := []byte(`
[general]
allow_subshells = false
allow_backticks = true

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if cfg.General.AllowSubshells {
			t.Error("AllowSubshells should be false")
		}
		if !cfg.General.AllowBackticks {
			t.Error("AllowBackticks should be true")
		}
	})

	t.Run("general section allow both", func(t *testing.T) {
		tomlData := []byte(`
[general]
allow_subshells = true
allow_backticks = true

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if !cfg.General.AllowSubshells {
			t.Error("AllowSubshells should be true")
		}
		if !cfg.General.AllowBackticks {
			t.Error("AllowBackticks should be true")
		}
	})

	t.Run("general section allowed_subshell_commands", func(t *testing.T) {
		tomlData := []byte(`
[general]
allowed_subshell_commands = ["cat", "pwd", "head"]

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		cfg, err := config.LoadConfig(tomlData)
		if err != nil {
			t.Errorf("LoadConfig() error = %v", err)
		}
		if len(cfg.General.AllowedSubshellCommands) != 3 {
			t.Errorf("AllowedSubshellCommands should have 3 items, got %d", len(cfg.General.AllowedSubshellCommands))
		}
		expected := []string{"cat", "pwd", "head"}
		for i, cmd := range expected {
			if cfg.General.AllowedSubshellCommands[i] != cmd {
				t.Errorf("AllowedSubshellCommands[%d] = %q, want %q", i, cfg.General.AllowedSubshellCommands[i], cmd)
			}
		}
	})
}

func TestGeneralSettings(t *testing.T) {
	t.Run("subshells rejected by default", func(t *testing.T) {
		config.Reset()
		tmpDir, err := os.MkdirTemp("", "mmi-sec-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configToml := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", tmpDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		config.Init()

		input := `{"tool_name":"Bash","tool_input":{"command":"echo $(whoami)"}}`
		approved, _ := hook.Process(strings.NewReader(input))
		if approved {
			t.Error("Subshells should be rejected by default")
		}
	})

	t.Run("backticks rejected by default", func(t *testing.T) {
		config.Reset()
		tmpDir, err := os.MkdirTemp("", "mmi-sec-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configToml := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", tmpDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		config.Init()

		input := "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"echo `whoami`\"}}"
		approved, _ := hook.Process(strings.NewReader(input))
		if approved {
			t.Error("Backticks should be rejected by default")
		}
	})

	t.Run("subshells allowed when configured", func(t *testing.T) {
		config.Reset()
		tmpDir, err := os.MkdirTemp("", "mmi-sec-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configToml := []byte(`
[general]
allow_subshells = true

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", tmpDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		config.Init()

		input := `{"tool_name":"Bash","tool_input":{"command":"echo $(pwd)"}}`
		approved, _ := hook.Process(strings.NewReader(input))
		if !approved {
			t.Error("Subshells should be allowed when configured")
		}
	})

	t.Run("backticks allowed when configured", func(t *testing.T) {
		config.Reset()
		tmpDir, err := os.MkdirTemp("", "mmi-sec-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configToml := []byte(`
[general]
allow_backticks = true

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", tmpDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		config.Init()

		input := "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"echo `pwd`\"}}"
		approved, _ := hook.Process(strings.NewReader(input))
		if !approved {
			t.Error("Backticks should be allowed when configured")
		}
	})

	t.Run("allowed_subshell_commands allows specific subshells", func(t *testing.T) {
		config.Reset()
		tmpDir, err := os.MkdirTemp("", "mmi-sec-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configToml := []byte(`
[general]
allowed_subshell_commands = ["cat", "pwd"]

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", tmpDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		config.Init()

		// cat should be allowed
		input := `{"tool_name":"Bash","tool_input":{"command":"echo $(cat file.txt)"}}`
		approved, _ := hook.Process(strings.NewReader(input))
		if !approved {
			t.Error("$(cat ...) should be allowed when cat is in allowed_subshell_commands")
		}

		// pwd should be allowed
		input = `{"tool_name":"Bash","tool_input":{"command":"echo $(pwd)"}}`
		approved, _ = hook.Process(strings.NewReader(input))
		if !approved {
			t.Error("$(pwd) should be allowed when pwd is in allowed_subshell_commands")
		}

		// whoami should be rejected
		input = `{"tool_name":"Bash","tool_input":{"command":"echo $(whoami)"}}`
		approved, _ = hook.Process(strings.NewReader(input))
		if approved {
			t.Error("$(whoami) should be rejected when whoami is not in allowed_subshell_commands")
		}
	})

	t.Run("allowed_subshell_commands works for backticks too", func(t *testing.T) {
		config.Reset()
		tmpDir, err := os.MkdirTemp("", "mmi-sec-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configToml := []byte(`
[general]
allowed_subshell_commands = ["cat"]

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", tmpDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		config.Init()

		// cat in backticks should be allowed
		input := "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"echo `cat file.txt`\"}}"
		approved, _ := hook.Process(strings.NewReader(input))
		if !approved {
			t.Error("`cat ...` should be allowed when cat is in allowed_subshell_commands")
		}

		// whoami in backticks should be rejected
		input = "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"echo `whoami`\"}}"
		approved, _ = hook.Process(strings.NewReader(input))
		if approved {
			t.Error("`whoami` should be rejected when whoami is not in allowed_subshell_commands")
		}
	})

	t.Run("multiple subshells must all be allowed", func(t *testing.T) {
		config.Reset()
		tmpDir, err := os.MkdirTemp("", "mmi-sec-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configToml := []byte(`
[general]
allowed_subshell_commands = ["cat"]

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)

		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", tmpDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		config.Init()

		// Mixed: cat allowed, whoami not - should reject
		input := `{"tool_name":"Bash","tool_input":{"command":"echo $(cat file) $(whoami)"}}`
		approved, _ := hook.Process(strings.NewReader(input))
		if approved {
			t.Error("Command with both allowed and disallowed subshells should be rejected")
		}

		// Both allowed - should pass
		configToml = []byte(`
[general]
allowed_subshell_commands = ["cat", "pwd"]

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
		os.WriteFile(filepath.Join(tmpDir, "config.toml"), configToml, 0644)
		config.Reset()
		config.Init()

		input = `{"tool_name":"Bash","tool_input":{"command":"echo $(cat file) $(pwd)"}}`
		approved, _ = hook.Process(strings.NewReader(input))
		if !approved {
			t.Error("Command with multiple allowed subshells should be approved")
		}
	})
}

func TestInitConfig(t *testing.T) {
	// Reset config state
	config.Reset()

	tmpDir, err := os.MkdirTemp("", "mmi-init-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origEnv := os.Getenv("MMI_CONFIG")
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Setenv("MMI_CONFIG", origEnv)

	// Init returns error when config doesn't exist
	err = config.Init()
	if err == nil {
		t.Error("Init() should return error when config file doesn't exist")
	}

	// Config should be empty (deny all) when no config file exists
	cfg := config.Get()
	if len(cfg.WrapperPatterns) != 0 {
		t.Error("WrapperPatterns should be empty when no config file exists")
	}
	if len(cfg.SafeCommands) != 0 {
		t.Error("SafeCommands should be empty when no config file exists")
	}

	// Config file should NOT be auto-created
	if _, err := os.Stat(filepath.Join(tmpDir, "config.toml")); err == nil {
		t.Error("config.toml should not be auto-created")
	}
}

func TestConfigCustomization(t *testing.T) {
	// Reset config state
	config.Reset()

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

	if err := config.Init(); err != nil {
		t.Errorf("Init() error = %v", err)
	}

	cfg := config.Get()

	// Verify custom patterns work
	core, wrappers := hook.StripWrappers("custom mycommand arg", cfg.WrapperPatterns)
	if len(wrappers) != 1 || wrappers[0] != "custom" {
		t.Errorf("Custom wrapper not stripped: %v", wrappers)
	}
	if hook.CheckSafe(core, cfg.SafeCommands) != "mycommand" {
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
