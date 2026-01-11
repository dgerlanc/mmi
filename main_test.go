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
)

// TestMain sets up config for all tests
func TestMain(m *testing.M) {
	// Create a temp directory for config during tests
	tmpDir, err := os.MkdirTemp("", "mmi-test-*")
	if err != nil {
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	// Set MMI_CONFIG to temp directory
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	// Initialize config (creates default config files in temp dir)
	initConfig()

	os.Exit(m.Run())
}

// =============================================================================
// main() Tests
// =============================================================================

func TestMainFunction(t *testing.T) {
	// Save original stdin and osExit
	origStdin := os.Stdin
	origExit := osExit
	defer func() {
		os.Stdin = origStdin
		osExit = origExit
	}()

	// Create a pipe to provide input
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Write test input
	go func() {
		w.WriteString(`{"tool_name":"Bash","tool_input":{"command":"git status"}}`)
		w.Close()
	}()

	// Replace stdin
	os.Stdin = r

	// Capture exit code
	var exitCode int
	osExit = func(code int) {
		exitCode = code
		// Don't actually exit during tests
	}

	// Call main
	main()

	if exitCode != 0 {
		t.Errorf("main() exitCode = %d, want 0", exitCode)
	}
}

// =============================================================================
// run() Tests
// =============================================================================

func TestRun(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectOutput bool // whether we expect JSON output
	}{
		// Safe command - should produce output
		{"safe command", `{"tool_name":"Bash","tool_input":{"command":"git status"}}`, true},
		// Unsafe command - no output
		{"unsafe command", `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`, false},
		// Non-Bash tool - no output
		{"non-Bash tool", `{"tool_name":"Read","tool_input":{"file":"/etc/passwd"}}`, false},
		// Invalid JSON - no output
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
				// Verify it's valid JSON
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

// =============================================================================
// process() Tests
// =============================================================================

func TestProcess(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectApproved bool
		expectReason   string
	}{
		// Safe commands
		{"simple git status", `{"tool_name":"Bash","tool_input":{"command":"git status"}}`, true, "git read op"},
		{"pytest", `{"tool_name":"Bash","tool_input":{"command":"pytest"}}`, true, "pytest"},
		{"chained safe", `{"tool_name":"Bash","tool_input":{"command":"git add . && git status"}}`, true, "git write op | git read op"},
		{"with wrapper", `{"tool_name":"Bash","tool_input":{"command":"timeout 30 pytest -v"}}`, true, "timeout + pytest"},
		{"env vars wrapper", `{"tool_name":"Bash","tool_input":{"command":"FOO=bar pytest"}}`, true, "env vars + pytest"},
		{"complex chain", `{"tool_name":"Bash","tool_input":{"command":"git status && pytest && echo done"}}`, true, "git read op | pytest | echo"},
		{"venv python", `{"tool_name":"Bash","tool_input":{"command":".venv/bin/python script.py"}}`, true, ".venv + python"},

		// Unsafe commands
		{"dangerous $()", `{"tool_name":"Bash","tool_input":{"command":"echo $(whoami)"}}`, false, ""},
		{"dangerous backtick", "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"echo `whoami`\"}}", false, ""},
		{"unsafe rm", `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`, false, ""},
		{"unsafe in chain", `{"tool_name":"Bash","tool_input":{"command":"git status && rm -rf /"}}`, false, ""},
		{"sudo", `{"tool_name":"Bash","tool_input":{"command":"sudo anything"}}`, false, ""},

		// Non-Bash tool
		{"non-Bash tool", `{"tool_name":"Read","tool_input":{"file":"/etc/passwd"}}`, false, ""},

		// Invalid JSON
		{"invalid JSON", "invalid json {{{", false, ""},

		// Empty command (gets approved with empty reason since no unsafe segments)
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

// =============================================================================
// formatApproval() Tests
// =============================================================================

func TestFormatApproval(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{"simple reason", "pytest"},
		{"complex reason", "timeout + pytest | git read op"},
		{"empty reason", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatApproval(tt.reason)

			// Should end with newline
			if !strings.HasSuffix(result, "\n") {
				t.Error("formatApproval() should end with newline")
			}

			// Should be valid JSON (without trailing newline)
			var output HookOutput
			if err := json.Unmarshal([]byte(strings.TrimSuffix(result, "\n")), &output); err != nil {
				t.Errorf("formatApproval() returned invalid JSON: %v", err)
				return
			}

			// Verify structure
			if output.HookSpecificOutput.HookEventName != "PreToolUse" {
				t.Errorf("HookEventName = %q, want %q", output.HookSpecificOutput.HookEventName, "PreToolUse")
			}
			if output.HookSpecificOutput.PermissionDecision != "allow" {
				t.Errorf("PermissionDecision = %q, want %q", output.HookSpecificOutput.PermissionDecision, "allow")
			}
			if output.HookSpecificOutput.PermissionDecisionReason != tt.reason {
				t.Errorf("PermissionDecisionReason = %q, want %q", output.HookSpecificOutput.PermissionDecisionReason, tt.reason)
			}
		})
	}
}

// =============================================================================
// splitCommandChain() Tests
// =============================================================================

func TestSplitCommandChain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		// Basic cases
		{"simple command", "ls -la", []string{"ls -la"}},
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},

		// Command separators
		{"AND chain", "cmd1 && cmd2", []string{"cmd1", "cmd2"}},
		{"OR chain", "cmd1 || cmd2", []string{"cmd1", "cmd2"}},
		{"semicolon chain", "cmd1 ; cmd2", []string{"cmd1", "cmd2"}},
		{"pipe", "cmd1 | cmd2", []string{"cmd1", "cmd2"}},
		{"background", "cmd1 & cmd2", []string{"cmd1", "cmd2"}},
		{"multiple separators", "a && b || c ; d | e", []string{"a", "b", "c", "d", "e"}},

		// Quoted string preservation
		{"double-quoted AND", `echo "a && b"`, []string{`echo "a && b"`}},
		{"single-quoted AND", `echo 'a && b'`, []string{`echo 'a && b'`}},
		{"double-quoted OR", `echo "a || b"`, []string{`echo "a || b"`}},
		{"single-quoted semicolon", `echo 'a ; b'`, []string{`echo 'a ; b'`}},
		{"double-quoted pipe", `echo "a | b"`, []string{`echo "a | b"`}},
		{"mixed quotes", `echo "a" && echo 'b'`, []string{`echo "a"`, `echo 'b'`}},
		{"nested quotes", `echo "a 'b' c"`, []string{`echo "a 'b' c"`}},

		// Redirections
		{"redirection 2>&1", "cmd 2>&1", []string{"cmd 2>&1"}},
		{"redirection &>", "cmd &> file", []string{"cmd &> file"}},
		{"redirection >&2", "cmd >&2", []string{"cmd >&2"}},
		{"multiple redirections", "cmd 2>&1 >&2", []string{"cmd 2>&1 >&2"}},
		{"redirection with chain", "cmd1 2>&1 && cmd2", []string{"cmd1 2>&1", "cmd2"}},

		// Backslash continuations
		{"backslash newline", "cmd \\\n arg", []string{"cmd  arg"}},
		{"backslash newline with space", "cmd \\\n    continued", []string{"cmd  continued"}},

		// Newline handling
		{"newline splits without quotes", "cmd1\ncmd2", []string{"cmd1", "cmd2"}},

		// Complex cases
		{"complex mixed", `echo "test" && ls | grep foo`, []string{`echo "test"`, "ls", "grep foo"}},
		{"real world git", "git add . && git commit -m 'test'", []string{"git add .", "git commit -m 'test'"}},
		{"pytest with options", "pytest -v tests/ && echo done", []string{"pytest -v tests/", "echo done"}},
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

// =============================================================================
// stripWrappers() Tests
// =============================================================================

func TestStripWrappers(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedCore     string
		expectedWrappers []string
	}{
		// No wrappers
		{"no wrapper", "pytest", "pytest", nil},
		{"no wrapper with args", "pytest -v tests/", "pytest -v tests/", nil},

		// timeout wrapper
		{"timeout", "timeout 30 pytest", "pytest", []string{"timeout"}},
		{"timeout with args", "timeout 60 pytest -v", "pytest -v", []string{"timeout"}},

		// nice wrapper
		{"nice", "nice pytest", "pytest", []string{"nice"}},
		{"nice -n", "nice -n 10 pytest", "pytest", []string{"nice"}},
		{"nice -n compact", "nice -n10 pytest", "pytest", []string{"nice"}},

		// env wrapper
		{"env", "env pytest", "pytest", []string{"env"}},
		{"env with command", "env python script.py", "python script.py", []string{"env"}},

		// env vars wrapper
		{"env vars single", "FOO=bar pytest", "pytest", []string{"env vars"}},
		{"env vars multiple", "FOO=1 BAR=2 pytest", "pytest", []string{"env vars"}},
		{"env vars with underscore", "MY_VAR=value pytest", "pytest", []string{"env vars"}},
		{"env vars complex value", "PATH=/usr/bin pytest", "pytest", []string{"env vars"}},

		// .venv/bin/ wrapper
		{".venv/bin/", ".venv/bin/python", "python", []string{".venv"}},
		{".venv/bin/ with args", ".venv/bin/python script.py", "python script.py", []string{".venv"}},
		{"../.venv/bin/", "../.venv/bin/python", "python", []string{".venv"}},
		{"../../.venv/bin/", "../../.venv/bin/pytest", "pytest", []string{".venv"}},
		{"venv/bin/ no dot", "venv/bin/python", "python", []string{".venv"}},
		{"absolute venv", "/home/user/.venv/bin/python", "python", []string{".venv"}},
		{"absolute venv no dot", "/project/venv/bin/pytest", "pytest", []string{".venv"}},

		// do wrapper (loop body)
		{"do", "do echo test", "echo test", []string{"do"}},
		{"do with command", "do pytest -v", "pytest -v", []string{"do"}},

		// Chained wrappers
		{"timeout + nice", "timeout 30 nice pytest", "pytest", []string{"timeout", "nice"}},
		{"env vars + timeout", "FOO=bar timeout 30 pytest", "pytest", []string{"env vars", "timeout"}},
		{"env + venv", "env .venv/bin/python", "python", []string{"env", ".venv"}},
		{"timeout + env vars + cmd", "timeout 60 FOO=1 BAR=2 pytest", "pytest", []string{"timeout", "env vars"}},
		{"nice + timeout + venv", "nice timeout 30 .venv/bin/pytest", "pytest", []string{"nice", "timeout", ".venv"}},
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
// checkSafe() Tests
// =============================================================================

func TestCheckSafe(t *testing.T) {
	// Test cases grouped by category
	safeTests := []struct {
		name     string
		input    string
		expected string // non-empty means safe
	}{
		// Git read operations
		{"git diff", "git diff", "git read op"},
		{"git diff with path", "git diff src/", "git read op"},
		{"git log", "git log", "git read op"},
		{"git log -n", "git log -n 10", "git read op"},
		{"git status", "git status", "git read op"},
		{"git show", "git show HEAD", "git read op"},
		{"git branch", "git branch", "git read op"},
		{"git branch -a", "git branch -a", "git read op"},
		{"git stash list", "git stash list", "git read op"},
		{"git bisect", "git bisect good", "git read op"},
		{"git worktree list", "git worktree list", "git read op"},
		{"git fetch", "git fetch origin", "git read op"},
		{"git -C diff", "git -C /path diff", "git read op"},
		{"git -C log", "git -C /project log --oneline", "git read op"},

		// Git write operations
		{"git add", "git add .", "git write op"},
		{"git add file", "git add file.txt", "git write op"},
		{"git checkout", "git checkout main", "git write op"},
		{"git checkout -b", "git checkout -b feature", "git write op"},
		{"git merge", "git merge feature", "git write op"},
		{"git rebase", "git rebase main", "git write op"},
		{"git stash", "git stash", "git write op"},
		{"git stash pop", "git stash pop", "git write op"},
		{"git -C add", "git -C /path add .", "git write op"},

		// pytest
		{"pytest", "pytest", "pytest"},
		{"pytest -v", "pytest -v", "pytest"},
		{"pytest path", "pytest tests/", "pytest"},
		{"pytest -x", "pytest -x --tb=short", "pytest"},

		// python
		{"python", "python", "python"},
		{"python script", "python script.py", "python"},
		{"python -m", "python -m pytest", "python"},
		{"python -c", "python -c 'print(1)'", "python"},

		// ruff
		{"ruff", "ruff", "ruff"},
		{"ruff check", "ruff check .", "ruff"},
		{"ruff format", "ruff format src/", "ruff"},

		// uv commands
		{"uv pip", "uv pip install package", "uv"},
		{"uv run", "uv run pytest", "uv"},
		{"uv sync", "uv sync", "uv"},
		{"uv venv", "uv venv", "uv"},
		{"uv add", "uv add package", "uv"},
		{"uv remove", "uv remove package", "uv"},
		{"uv lock", "uv lock", "uv"},

		// uvx
		{"uvx", "uvx", "uvx"},
		{"uvx tool", "uvx ruff check", "uvx"},

		// npm commands
		{"npm install", "npm install", "npm"},
		{"npm run", "npm run build", "npm"},
		{"npm test", "npm test", "npm"},
		{"npm build", "npm build", "npm"},
		{"npm ci", "npm ci", "npm"},

		// npx
		{"npx", "npx", "npx"},
		{"npx tool", "npx eslint .", "npx"},

		// cargo commands
		{"cargo build", "cargo build", "cargo"},
		{"cargo build release", "cargo build --release", "cargo"},
		{"cargo test", "cargo test", "cargo"},
		{"cargo run", "cargo run", "cargo"},
		{"cargo check", "cargo check", "cargo"},
		{"cargo clippy", "cargo clippy", "cargo"},
		{"cargo fmt", "cargo fmt", "cargo"},
		{"cargo clean", "cargo clean", "cargo"},

		// maturin
		{"maturin develop", "maturin develop", "maturin"},
		{"maturin build", "maturin build", "maturin"},

		// make
		{"make", "make", "make"},
		{"make target", "make build", "make"},
		{"make with var", "make VERBOSE=1", "make"},

		// Read-only commands
		{"ls", "ls", "read-only"},
		{"ls -la", "ls -la", "read-only"},
		{"cat", "cat file.txt", "read-only"},
		{"head", "head -n 10 file", "read-only"},
		{"tail", "tail -f log", "read-only"},
		{"wc", "wc -l file", "read-only"},
		{"find", "find . -name '*.go'", "read-only"},
		{"grep", "grep pattern file", "read-only"},
		{"rg", "rg pattern", "read-only"},
		{"file", "file binary", "read-only"},
		{"which", "which python", "read-only"},
		{"pwd", "pwd", "read-only"},
		{"du", "du -sh .", "read-only"},
		{"df", "df -h", "read-only"},
		{"curl", "curl https://example.com", "read-only"},
		{"sort", "sort file", "read-only"},
		{"uniq", "uniq file", "read-only"},
		{"cut", "cut -d: -f1 file", "read-only"},
		{"tr", "tr a-z A-Z", "read-only"},
		{"awk", "awk '{print $1}'", "read-only"},
		{"sed", "sed 's/a/b/g'", "read-only"},
		{"xargs", "xargs echo", "read-only"},

		// touch
		{"touch", "touch file.txt", "touch"},
		{"touch multiple", "touch a.txt b.txt", "touch"},

		// Shell builtins
		{"true", "true", "shell builtin"},
		{"false", "false", "shell builtin"},
		{"exit", "exit", "shell builtin"},
		{"exit 0", "exit 0", "shell builtin"},
		{"exit 1", "exit 1", "shell builtin"},

		// Process management
		{"pkill", "pkill process", "process mgmt"},
		{"kill", "kill 1234", "process mgmt"},
		{"kill -9", "kill -9 1234", "process mgmt"},

		// echo
		{"echo", "echo", "echo"},
		{"echo message", "echo hello world", "echo"},
		{"echo -n", "echo -n test", "echo"},

		// cd
		{"cd dir", "cd dir", "cd"},
		{"cd path", "cd /path/to/dir", "cd"},
		{"cd home", "cd ~", "cd"},

		// venv activate
		{"source venv activate", "source .venv/bin/activate", "venv activate"},
		{"dot venv activate", ". venv/bin/activate", "venv activate"},
		{"source no dot", "source venv/bin/activate", "venv activate"},

		// sleep
		{"sleep 1", "sleep 1", "sleep"},
		{"sleep 30", "sleep 30", "sleep"},

		// Variable assignment
		{"var assignment", "FOO=bar", "var assignment"},
		{"var assignment special", "VAR=$!", "var assignment"},
		{"var assignment number", "COUNT=0", "var assignment"},

		// Loop constructs
		{"for loop", "for x in a b c", "for loop"},
		{"for loop files", "for f in *.txt", "for loop"},
		{"while loop", "while true", "while loop"},
		{"while read", "while read line", "while loop"},
		{"done", "done", "done"},
	}

	for _, tt := range safeTests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkSafe(tt.input)
			if got != tt.expected {
				t.Errorf("checkSafe(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCheckSafeUnsafeCommands(t *testing.T) {
	unsafeCommands := []struct {
		name  string
		input string
	}{
		{"rm", "rm file"},
		{"rm -rf", "rm -rf /"},
		{"sudo", "sudo anything"},
		{"eval", "eval code"},
		{"exec", "exec command"},
		{"bash -c", "bash -c 'cmd'"},
		{"sh script", "sh script.sh"},
		{"chmod", "chmod 777 file"},
		{"chown", "chown user file"},
		{"mv", "mv file1 file2"},
		{"cp", "cp file1 file2"},
		{"mkdir", "mkdir dir"},
		{"rmdir", "rmdir dir"},
		{"wget", "wget url"},
		{"apt", "apt install pkg"},
		{"yum", "yum install pkg"},
		{"brew", "brew install pkg"},
		{"pip direct", "pip install pkg"},
		{"unknown command", "unknowncommand arg"},
		{"empty", ""},
	}

	for _, tt := range unsafeCommands {
		t.Run(tt.name, func(t *testing.T) {
			got := checkSafe(tt.input)
			if got != "" {
				t.Errorf("checkSafe(%q) = %q, want empty string (unsafe)", tt.input, got)
			}
		})
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

// Helper to run the binary with input and capture output
func runMmi(t *testing.T, input string) (string, int) {
	t.Helper()

	// Build the binary if needed
	cmd := exec.Command("go", "build", "-o", "mmi_test_binary", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build: %v", err)
	}
	defer os.Remove("mmi_test_binary")

	// Run with input
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

func TestIntegrationSafeCommands(t *testing.T) {
	tests := []struct {
		name           string
		command        string
		expectApproval bool
		expectReason   string
	}{
		{"simple git status", "git status", true, "git read op"},
		{"pytest", "pytest", true, "pytest"},
		{"chained safe", "git add . && git status", true, "git write op | git read op"},
		{"with wrapper", "timeout 30 pytest -v", true, "timeout + pytest"},
		{"env vars wrapper", "FOO=bar pytest", true, "env vars + pytest"},
		{"complex chain", "git status && pytest && echo done", true, "git read op | pytest | echo"},
		{"venv python", ".venv/bin/python script.py", true, ".venv + python"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := HookInput{
				ToolName:  "Bash",
				ToolInput: map[string]string{"command": tt.command},
			}
			inputJSON, _ := json.Marshal(input)

			output, exitCode := runMmi(t, string(inputJSON))

			if tt.expectApproval {
				if exitCode != 0 {
					t.Errorf("Expected exit 0 for approval, got %d", exitCode)
				}
				if output == "" {
					t.Error("Expected approval output, got empty")
					return
				}

				var result HookOutput
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Errorf("Failed to parse output: %v", err)
					return
				}

				if result.HookSpecificOutput.PermissionDecision != "allow" {
					t.Errorf("Expected 'allow', got %q", result.HookSpecificOutput.PermissionDecision)
				}
				if !strings.Contains(result.HookSpecificOutput.PermissionDecisionReason, tt.expectReason) {
					t.Errorf("Expected reason to contain %q, got %q",
						tt.expectReason, result.HookSpecificOutput.PermissionDecisionReason)
				}
			} else {
				if output != "" {
					t.Errorf("Expected no output for rejection, got %q", output)
				}
			}
		})
	}
}

func TestIntegrationUnsafeCommands(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"dangerous $()", "echo $(whoami)"},
		{"dangerous backtick", "echo `whoami`"},
		{"unsafe rm", "rm -rf /"},
		{"unsafe in chain", "git status && rm -rf /"},
		{"sudo", "sudo anything"},
		{"eval", "eval dangerous"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := HookInput{
				ToolName:  "Bash",
				ToolInput: map[string]string{"command": tt.command},
			}
			inputJSON, _ := json.Marshal(input)

			output, _ := runMmi(t, string(inputJSON))

			if output != "" {
				t.Errorf("Expected no output for unsafe command %q, got %q", tt.command, output)
			}
		})
	}
}

func TestIntegrationNonBashTool(t *testing.T) {
	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]string{"file": "/etc/passwd"},
	}
	inputJSON, _ := json.Marshal(input)

	output, _ := runMmi(t, string(inputJSON))

	if output != "" {
		t.Errorf("Expected no output for non-Bash tool, got %q", output)
	}
}

func TestIntegrationInvalidJSON(t *testing.T) {
	output, exitCode := runMmi(t, "invalid json {{{")

	if output != "" {
		t.Errorf("Expected no output for invalid JSON, got %q", output)
	}
	if exitCode != 0 {
		t.Errorf("Expected exit 0 for invalid JSON, got %d", exitCode)
	}
}

func TestIntegrationEmptyCommand(t *testing.T) {
	input := HookInput{
		ToolName:  "Bash",
		ToolInput: map[string]string{"command": ""},
	}
	inputJSON, _ := json.Marshal(input)

	output, exitCode := runMmi(t, string(inputJSON))

	// Empty command currently gets approved with empty reason
	// (this is the actual behavior - splitCommandChain returns nil for empty input)
	if exitCode != 0 {
		t.Errorf("Expected exit 0, got %d", exitCode)
	}
	if output == "" {
		t.Error("Expected approval output for empty command")
	}
}

// =============================================================================
// Edge Cases & Security Tests
// =============================================================================

func TestDangerousPatternInQuotes(t *testing.T) {
	// Even quoted dangerous patterns should be rejected
	// because the check happens on the raw command string
	tests := []string{
		`echo "$(whoami)"`,
		"echo '$(whoami)'",
		"echo \"`whoami`\"",
	}

	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			if !dangerousPattern.MatchString(cmd) {
				t.Errorf("Expected dangerous pattern to match %q", cmd)
			}
		})
	}
}

func TestWhitespaceVariations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"extra spaces around &&", "cmd1   &&   cmd2", []string{"cmd1", "cmd2"}},
		{"tabs", "cmd1\t&&\tcmd2", []string{"cmd1", "cmd2"}},
		{"leading whitespace", "   git status", []string{"git status"}},
		{"trailing whitespace", "git status   ", []string{"git status"}},
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

func TestLongCommand(t *testing.T) {
	// Build a very long but safe command
	var parts []string
	for i := 0; i < 100; i++ {
		parts = append(parts, "echo test")
	}
	longCmd := strings.Join(parts, " && ")

	segments := splitCommandChain(longCmd)
	if len(segments) != 100 {
		t.Errorf("Expected 100 segments, got %d", len(segments))
	}

	// Each segment should be safe
	for _, seg := range segments {
		if checkSafe(seg) == "" {
			t.Errorf("Expected %q to be safe", seg)
		}
	}
}

// =============================================================================
// Config Tests
// =============================================================================

func TestGetConfigDir(t *testing.T) {
	// Test with MMI_CONFIG set
	t.Run("with MMI_CONFIG env var", func(t *testing.T) {
		origVal := os.Getenv("MMI_CONFIG")
		defer os.Setenv("MMI_CONFIG", origVal)

		os.Setenv("MMI_CONFIG", "/custom/path")
		dir, err := getConfigDir()
		if err != nil {
			t.Errorf("getConfigDir() error = %v", err)
		}
		if dir != "/custom/path" {
			t.Errorf("getConfigDir() = %q, want %q", dir, "/custom/path")
		}
	})

	// Test without MMI_CONFIG (uses default)
	t.Run("without MMI_CONFIG env var", func(t *testing.T) {
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

	// First call should create files
	err = ensureConfigFiles(configDir)
	if err != nil {
		t.Errorf("ensureConfigFiles() error = %v", err)
	}

	// Check files exist
	wrappersPath := filepath.Join(configDir, "wrappers.toml")
	commandsPath := filepath.Join(configDir, "commands.toml")

	if _, err := os.Stat(wrappersPath); os.IsNotExist(err) {
		t.Error("wrappers.toml was not created")
	}
	if _, err := os.Stat(commandsPath); os.IsNotExist(err) {
		t.Error("commands.toml was not created")
	}

	// Second call should not overwrite existing files
	originalContent, _ := os.ReadFile(wrappersPath)
	err = ensureConfigFiles(configDir)
	if err != nil {
		t.Errorf("second ensureConfigFiles() error = %v", err)
	}
	newContent, _ := os.ReadFile(wrappersPath)
	if !bytes.Equal(originalContent, newContent) {
		t.Error("ensureConfigFiles() overwrote existing file")
	}
}

func TestLoadPatterns(t *testing.T) {
	t.Run("valid TOML", func(t *testing.T) {
		tomlData := []byte(`
[[wrapper]]
pattern = "^test\\s+"
name = "test"

[[wrapper]]
pattern = "^foo\\s+"
name = "foo"
`)
		patterns, err := loadPatterns(tomlData, "wrapper")
		if err != nil {
			t.Errorf("loadPatterns() error = %v", err)
		}
		if len(patterns) != 2 {
			t.Errorf("loadPatterns() returned %d patterns, want 2", len(patterns))
		}
		if patterns[0].Name != "test" {
			t.Errorf("patterns[0].Name = %q, want %q", patterns[0].Name, "test")
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		tomlData := []byte(`
[[wrapper]]
pattern = "[invalid"
name = "bad"
`)
		_, err := loadPatterns(tomlData, "wrapper")
		if err == nil {
			t.Error("loadPatterns() should return error for invalid regex")
		}
	})

	t.Run("invalid TOML", func(t *testing.T) {
		tomlData := []byte(`this is not valid toml {{{}}}`)
		_, err := loadPatterns(tomlData, "wrapper")
		if err == nil {
			t.Error("loadPatterns() should return error for invalid TOML")
		}
	})
}

func TestInitConfig(t *testing.T) {
	// Save original state
	origWrappers := wrapperPatterns
	origCommands := safeCommands
	origInitialized := configInitialized
	defer func() {
		wrapperPatterns = origWrappers
		safeCommands = origCommands
		configInitialized = origInitialized
	}()

	t.Run("creates config files in new directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "mmi-init-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		configDir := filepath.Join(tmpDir, "mmi")

		// Set MMI_CONFIG
		origEnv := os.Getenv("MMI_CONFIG")
		os.Setenv("MMI_CONFIG", configDir)
		defer os.Setenv("MMI_CONFIG", origEnv)

		// Reset state
		configInitialized = false
		wrapperPatterns = nil
		safeCommands = nil

		err = initConfig()
		if err != nil {
			t.Errorf("initConfig() error = %v", err)
		}

		// Verify patterns were loaded
		if len(wrapperPatterns) == 0 {
			t.Error("wrapperPatterns is empty after initConfig()")
		}
		if len(safeCommands) == 0 {
			t.Error("safeCommands is empty after initConfig()")
		}

		// Verify files were created
		if _, err := os.Stat(filepath.Join(configDir, "wrappers.toml")); os.IsNotExist(err) {
			t.Error("wrappers.toml was not created")
		}
		if _, err := os.Stat(filepath.Join(configDir, "commands.toml")); os.IsNotExist(err) {
			t.Error("commands.toml was not created")
		}
	})
}

func TestConfigCustomization(t *testing.T) {
	// Save original state
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

	// Write custom config
	wrappersToml := []byte(`
[[wrapper]]
pattern = "^custom\\s+"
name = "custom"
`)
	commandsToml := []byte(`
[[command]]
pattern = "^mycommand\\b"
name = "mycommand"
`)
	os.WriteFile(filepath.Join(tmpDir, "wrappers.toml"), wrappersToml, 0644)
	os.WriteFile(filepath.Join(tmpDir, "commands.toml"), commandsToml, 0644)

	// Set MMI_CONFIG
	origEnv := os.Getenv("MMI_CONFIG")
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Setenv("MMI_CONFIG", origEnv)

	// Reset state and load
	configInitialized = false
	wrapperPatterns = nil
	safeCommands = nil

	err = initConfig()
	if err != nil {
		t.Errorf("initConfig() error = %v", err)
	}

	// Verify custom patterns were loaded
	if len(wrapperPatterns) != 1 || wrapperPatterns[0].Name != "custom" {
		t.Errorf("Custom wrapper pattern not loaded correctly: %v", wrapperPatterns)
	}
	if len(safeCommands) != 1 || safeCommands[0].Name != "mycommand" {
		t.Errorf("Custom command pattern not loaded correctly: %v", safeCommands)
	}

	// Verify custom patterns work
	core, wrappers := stripWrappers("custom mycommand arg")
	if len(wrappers) != 1 || wrappers[0] != "custom" {
		t.Errorf("Custom wrapper not stripped: wrappers=%v", wrappers)
	}
	if checkSafe(core) != "mycommand" {
		t.Errorf("Custom command not recognized as safe: %q", core)
	}
}
