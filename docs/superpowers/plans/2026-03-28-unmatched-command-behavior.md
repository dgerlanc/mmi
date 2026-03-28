# Unmatched Command Behavior Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a configurable `unmatched` setting (`"ask"`, `"passthrough"`, `"reject"`) that controls what MMI does when a command doesn't match any safe pattern.

**Architecture:** New `[defaults]` config section parsed alongside `[subshell]`. The `Config` struct gains an `Unmatched` field. The hook processing pipeline is unchanged except the final "no match" branch, which now consults `cfg.Unmatched`. Passthrough produces empty output (exit 0, no JSON). A new `PASSTHROUGH` audit code tracks abstained commands.

**Tech Stack:** Go, TOML config, JSON audit logging

---

### Task 1: Add `Unmatched` field to Config struct and parse `[defaults]` section

**Files:**
- Modify: `internal/config/config.go:21-32` (Config struct)
- Modify: `internal/config/config.go:224-318` (loadConfigWithIncludes)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests for config parsing**

Add these tests to `internal/config/config_test.go`:

```go
func TestLoadConfigUnmatchedDefaultsToAsk(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "ask" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "ask")
	}
}

func TestLoadConfigUnmatchedPassthrough(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "passthrough" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "passthrough")
	}
}

func TestLoadConfigUnmatchedReject(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "reject"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "reject" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "reject")
	}
}

func TestLoadConfigUnmatchedAskExplicit(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "ask"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Unmatched != "ask" {
		t.Errorf("Unmatched = %q, want %q", cfg.Unmatched, "ask")
	}
}

func TestLoadConfigUnmatchedInvalidValue(t *testing.T) {
	data := []byte(`
[defaults]
unmatched = "foo"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for invalid unmatched value")
	}
	if !strings.Contains(err.Error(), "unmatched") {
		t.Errorf("error should mention 'unmatched', got: %v", err)
	}
}

func TestLoadConfigUnmatchedIncludeOverride(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[defaults]
unmatched = "reject"
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	// Main config is processed after includes, so main's value wins
	if cfg.Unmatched != "passthrough" {
		t.Errorf("Unmatched = %q, want %q (main config should override include)", cfg.Unmatched, "passthrough")
	}
}

func TestLoadConfigUnmatchedFromInclude(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[defaults]
unmatched = "passthrough"
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	// Main config omits [defaults], so include's value is inherited
	// But because main's zero value ("") overwrites, it defaults to "ask"
	if cfg.Unmatched != "ask" {
		t.Errorf("Unmatched = %q, want %q (omitted main defaults to ask, overwriting include)", cfg.Unmatched, "ask")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run "TestLoadConfigUnmatched" -v`
Expected: FAIL — `cfg.Unmatched` is undefined

- [ ] **Step 3: Add `Unmatched` field to Config struct**

In `internal/config/config.go`, change the `Config` struct (lines 21-32):

```go
// Config holds the compiled patterns from configuration.
type Config struct {
	// WrapperPatterns are safe prefixes that can wrap commands
	WrapperPatterns []patterns.Pattern
	// SafeCommands are patterns for allowed commands
	SafeCommands []patterns.Pattern
	// DenyPatterns are patterns that are always rejected (checked before approval)
	DenyPatterns []patterns.Pattern
	// SubshellAllowAll when true skips command substitution rejection
	SubshellAllowAll bool
	// RewriteRules are patterns that trigger command rewrite suggestions
	RewriteRules []patterns.RewriteRule
	// Unmatched controls behavior for commands that don't match any safe pattern.
	// Valid values: "ask" (default), "passthrough", "reject"
	Unmatched string
}
```

- [ ] **Step 4: Add `[defaults]` parsing in `loadConfigWithIncludes`**

In `internal/config/config.go`, add a `validUnmatched` set and parsing logic. Add these constants near the top of the file (after the imports):

```go
// Valid values for the [defaults] unmatched setting
const (
	UnmatchedAsk         = "ask"
	UnmatchedPassthrough = "passthrough"
	UnmatchedReject      = "reject"
)
```

In `loadConfigWithIncludes`, after the include merging block (after line 274 where `cfg.RewriteRules` is appended), add this line to merge the included `Unmatched` value:

```go
cfg.Unmatched = includeCfg.Unmatched
```

Then after the `[subshell]` parsing block (after line 307), add:

```go
// Parse defaults section
if defaultsSection, ok := raw["defaults"].(map[string]any); ok {
	if unmatched, ok := defaultsSection["unmatched"].(string); ok {
		switch unmatched {
		case UnmatchedAsk, UnmatchedPassthrough, UnmatchedReject:
			cfg.Unmatched = unmatched
		default:
			return nil, fmt.Errorf("invalid [defaults] unmatched value %q: must be \"ask\", \"passthrough\", or \"reject\"", unmatched)
		}
	}
}
```

Finally, at the end of `loadConfigWithIncludes`, before `return cfg, nil`, normalize the empty default:

```go
// Default to "ask" if not set
if cfg.Unmatched == "" {
	cfg.Unmatched = UnmatchedAsk
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -run "TestLoadConfigUnmatched" -v`
Expected: All PASS

- [ ] **Step 6: Run full test suite to check for regressions**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add [defaults] unmatched config field with ask/passthrough/reject"
```

---

### Task 2: Add `PASSTHROUGH` audit code

**Files:**
- Modify: `internal/audit/audit.go:15-22`

- [ ] **Step 1: Add the `CodePassthrough` constant**

In `internal/audit/audit.go`, add `CodePassthrough` to the rejection codes block (line 22):

```go
// Rejection codes
const (
	CodeCommandSubstitution = "COMMAND_SUBSTITUTION"
	CodeUnparseable         = "UNPARSEABLE"
	CodeDenyMatch           = "DENY_MATCH"
	CodeNoMatch             = "NO_MATCH"
	CodeRewrite             = "REWRITE"
	CodePassthrough         = "PASSTHROUGH"
)
```

- [ ] **Step 2: Run tests to confirm no regressions**

Run: `go test ./internal/audit/ -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/audit/audit.go
git commit -m "feat: add PASSTHROUGH audit rejection code"
```

---

### Task 3: Update hook processing for `unmatched` behavior

**Files:**
- Modify: `internal/hook/hook.go:176-349` (ProcessWithResult)
- Modify: `internal/hook/hook.go:36-42` (Result struct)
- Test: `internal/hook/hook_test.go`

- [ ] **Step 1: Write failing tests for passthrough behavior**

Add these tests to `internal/hook/hook_test.go`:

```go
func TestProcessWithResultUnmatchedPassthrough(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "some_unknown_command"}
	}`

	result := ProcessWithResult(strings.NewReader(input))

	if result.Approved {
		t.Error("expected Approved = false for passthrough")
	}
	if !result.Passthrough {
		t.Error("expected Passthrough = true")
	}
	if result.Output != "" {
		t.Errorf("expected empty Output for passthrough, got %q", result.Output)
	}

	// Verify audit log has PASSTHROUGH code
	entry := readLastAuditEntry(t, logPath)
	if len(entry.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(entry.Segments))
	}
	if entry.Segments[0].Rejection == nil {
		t.Fatal("expected rejection in segment")
	}
	if entry.Segments[0].Rejection.Code != audit.CodePassthrough {
		t.Errorf("rejection code = %q, want %q", entry.Segments[0].Rejection.Code, audit.CodePassthrough)
	}
}

func TestProcessWithResultUnmatchedReject(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[defaults]
unmatched = "reject"

[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer cleanupConfig()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "some_unknown_command"}
	}`

	result := ProcessWithResult(strings.NewReader(input))

	if result.Approved {
		t.Error("expected Approved = false for reject")
	}
	if result.Passthrough {
		t.Error("expected Passthrough = false for reject mode")
	}
	if !strings.Contains(result.Output, `"permissionDecision":"deny"`) {
		t.Errorf("expected deny decision in output, got %q", result.Output)
	}
}

func TestProcessWithResultUnmatchedAskDefault(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer cleanupConfig()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "some_unknown_command"}
	}`

	result := ProcessWithResult(strings.NewReader(input))

	if result.Approved {
		t.Error("expected Approved = false for ask")
	}
	if result.Passthrough {
		t.Error("expected Passthrough = false for ask mode")
	}
	if !strings.Contains(result.Output, `"permissionDecision":"ask"`) {
		t.Errorf("expected ask decision in output, got %q", result.Output)
	}
}

func TestProcessWithResultPassthroughDenyStillBlocks(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[defaults]
unmatched = "passthrough"

[[deny.simple]]
name = "dangerous"
commands = ["rm"]

[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer cleanupConfig()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf /"}
	}`

	result := ProcessWithResult(strings.NewReader(input))

	if result.Approved {
		t.Error("expected Approved = false for deny match")
	}
	if result.Passthrough {
		t.Error("expected Passthrough = false when deny matched")
	}
	if !strings.Contains(result.Output, `"permissionDecision":"deny"`) {
		t.Errorf("expected deny decision, got %q", result.Output)
	}
}

func TestProcessWithResultPassthroughRewriteStillBlocks(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "safe"
commands = ["ls"]

[[rewrites.simple]]
name = "use uv"
match = ["python"]
replace = "uv run python"
`)
	defer cleanupConfig()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "python script.py"}
	}`

	result := ProcessWithResult(strings.NewReader(input))

	if result.Approved {
		t.Error("expected Approved = false for rewrite match")
	}
	if result.Passthrough {
		t.Error("expected Passthrough = false when rewrite matched")
	}
	if !strings.Contains(result.Output, `"permissionDecision":"deny"`) {
		t.Errorf("expected deny decision for rewrite, got %q", result.Output)
	}
}

func TestProcessWithResultPassthroughSafeStillApproves(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer cleanupConfig()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "ls -la"}
	}`

	result := ProcessWithResult(strings.NewReader(input))

	if !result.Approved {
		t.Error("expected Approved = true for safe command in passthrough mode")
	}
	if result.Passthrough {
		t.Error("expected Passthrough = false when command is safe")
	}
	if !strings.Contains(result.Output, `"permissionDecision":"allow"`) {
		t.Errorf("expected allow decision, got %q", result.Output)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/hook/ -run "TestProcessWithResultUnmatched|TestProcessWithResultPassthrough" -v`
Expected: FAIL — `Passthrough` field doesn't exist on `Result`

- [ ] **Step 3: Add `Passthrough` field to Result struct**

In `internal/hook/hook.go`, update the `Result` struct (lines 37-42):

```go
// Result contains the outcome of processing a command.
type Result struct {
	Command     string // The command that was processed
	Approved    bool   // Whether the command was approved
	Reason      string // The reason for approval/denial
	Output      string // The JSON output sent to Claude Code
	Passthrough bool   // Whether MMI abstained (no output, let Claude Code decide)
}
```

- [ ] **Step 4: Update the "no match" decision branch in ProcessWithResult**

In `internal/hook/hook.go`, replace the segment-level `CodeNoMatch` block (lines 296-305):

```go
		if !safeResult.Matched {
			logger.Debug("rejected unsafe command", "command", coreCmd)
			overallApproved = false
			rejCode := audit.CodeNoMatch
			if cfg.Unmatched == config.UnmatchedPassthrough {
				rejCode = audit.CodePassthrough
			}
			auditSegments = append(auditSegments, audit.Segment{
				Command:   segment,
				Approved:  false,
				Wrappers:  wrappers,
				Rejection: &audit.Rejection{Code: rejCode},
			})
			continue
		}
```

Then update the final decision block (lines 329-343). Replace:

```go
	if !overallApproved {
		var output string
		if hasDenyMatch {
			output = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"command matches deny list"}}`
		} else if hasRewrite {
			reason := strings.Join(rewriteSuggestions, "; ")
			output = FormatDeny(reason)
		} else {
			output = FormatAsk("command not in allow list")
		}
		logAudit(cmd, false, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd, rawInput, output)
		return Result{Command: cmd, Approved: false, Output: output}
	}
```

With:

```go
	if !overallApproved {
		var output string
		passthrough := false
		if hasDenyMatch {
			output = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"command matches deny list"}}`
		} else if hasRewrite {
			reason := strings.Join(rewriteSuggestions, "; ")
			output = FormatDeny(reason)
		} else {
			switch cfg.Unmatched {
			case config.UnmatchedPassthrough:
				output = ""
				passthrough = true
			case config.UnmatchedReject:
				output = FormatDeny("command not in allow list")
			default:
				output = FormatAsk("command not in allow list")
			}
		}
		logAudit(cmd, false, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd, rawInput, output)
		return Result{Command: cmd, Approved: false, Output: output, Passthrough: passthrough}
	}
```

- [ ] **Step 5: Add `config` import to hook.go**

In `internal/hook/hook.go`, add `"github.com/dgerlanc/mmi/internal/config"` to the import block if not already present. Check first — it's already imported (line 14).

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/hook/ -run "TestProcessWithResultUnmatched|TestProcessWithResultPassthrough" -v`
Expected: All PASS

- [ ] **Step 7: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/hook/hook.go internal/hook/hook_test.go
git commit -m "feat: implement unmatched command behavior (passthrough/reject/ask)"
```

---

### Task 4: Update `runHook` to handle passthrough output

**Files:**
- Modify: `cmd/run.go:12-30`
- Test: `cmd/run_test.go`

- [ ] **Step 1: Write failing test for passthrough in normal mode**

Add this test to `cmd/run_test.go`:

```go
func TestRunHookNormalModePassthrough(t *testing.T) {
	resetGlobalState()

	cleanup := testutil.SetupTestConfig(t, `
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer func() { cleanup(); resetGlobalState() }()

	dryRun = false

	input := `{"tool_name":"Bash","tool_input":{"command":"some_unknown_command"}}`

	oldStdin := os.Stdin
	oldStdout := os.Stdout

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stdoutW.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, stdoutR)
	output := buf.String()

	// Passthrough should produce no output
	if output != "" {
		t.Errorf("expected empty output for passthrough, got: %s", output)
	}
}

func TestRunHookDryRunPassthrough(t *testing.T) {
	resetGlobalState()

	cleanup := testutil.SetupTestConfig(t, `
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer func() { cleanup(); resetGlobalState() }()

	dryRun = true
	defer func() { dryRun = false }()

	input := `{"tool_name":"Bash","tool_input":{"command":"some_unknown_command"}}`

	oldStdin := os.Stdin
	oldStderr := os.Stderr

	stdinR, stdinW, _ := os.Pipe()
	stdinW.WriteString(input)
	stdinW.Close()
	os.Stdin = stdinR

	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	cmd := &cobra.Command{}
	runHook(cmd, []string{})

	os.Stdin = oldStdin
	stderrW.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, stderrR)
	output := buf.String()

	if !strings.Contains(output, "PASSTHROUGH") {
		t.Errorf("expected 'PASSTHROUGH' in dry-run output, got: %s", output)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run "TestRunHook.*Passthrough" -v`
Expected: FAIL — `runHook` doesn't handle passthrough yet

- [ ] **Step 3: Update `runHook` to handle passthrough**

In `cmd/run.go`, replace the `runHook` function:

```go
// runHook is the default command that processes stdin for command approval
func runHook(cmd *cobra.Command, args []string) {
	// Process the command
	result := hook.ProcessWithResult(os.Stdin)

	if dryRun {
		// In dry-run mode, output to stderr instead of JSON to stdout
		if result.Approved {
			fmt.Fprintf(os.Stderr, "APPROVED: %s (reason: %s)\n", result.Command, result.Reason)
		} else if result.Passthrough {
			fmt.Fprintf(os.Stderr, "PASSTHROUGH: %s\n", result.Command)
		} else if result.Command != "" {
			fmt.Fprintf(os.Stderr, "REJECTED: %s\n", result.Command)
		} else {
			fmt.Fprintf(os.Stderr, "REJECTED: (no command parsed)\n")
		}
		return
	}

	// Normal mode: output JSON decision to stdout
	// For passthrough, output nothing (empty output signals abstain to Claude Code)
	fmt.Print(result.Output)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run "TestRunHook.*Passthrough" -v`
Expected: All PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/run.go cmd/run_test.go
git commit -m "feat: handle passthrough in runHook and dry-run output"
```

---

### Task 5: Update validate command to show `unmatched` setting

**Files:**
- Modify: `cmd/validate.go:26-67`
- Test: `cmd/validate_test.go`

- [ ] **Step 1: Write failing test for validate output**

Add these tests to `cmd/validate_test.go`:

```go
func TestRunValidateShowsUnmatchedBehavior(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	testConfig := `
[defaults]
unmatched = "passthrough"

[[commands.simple]]
name = "test"
commands = ["ls"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	config.Reset()
	config.Init()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := runValidate(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	if !strings.Contains(output, "Unmatched command behavior: passthrough") {
		t.Errorf("expected 'Unmatched command behavior: passthrough' in output, got:\n%s", output)
	}
}

func TestRunValidateShowsUnmatchedDefaultAsk(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	testConfig := `
[[commands.simple]]
name = "test"
commands = ["ls"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	config.Reset()
	config.Init()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := runValidate(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	if !strings.Contains(output, "Unmatched command behavior: ask") {
		t.Errorf("expected 'Unmatched command behavior: ask' in output, got:\n%s", output)
	}
}

func TestRunValidateUnmatchedAppearsBeforeSubshell(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	testConfig := `
[defaults]
unmatched = "reject"

[[commands.simple]]
name = "test"
commands = ["ls"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	config.Reset()
	config.Init()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := runValidate(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	unmatchedIdx := strings.Index(output, "Unmatched command behavior:")
	subshellIdx := strings.Index(output, "Subshell allow all:")
	if unmatchedIdx == -1 {
		t.Fatal("'Unmatched command behavior:' not found in output")
	}
	if subshellIdx == -1 {
		t.Fatal("'Subshell allow all:' not found in output")
	}
	if unmatchedIdx >= subshellIdx {
		t.Error("'Unmatched command behavior' should appear before 'Subshell allow all'")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run "TestRunValidateShowsUnmatched|TestRunValidateUnmatched" -v`
Expected: FAIL — output doesn't contain "Unmatched command behavior"

- [ ] **Step 3: Update validate command output**

In `cmd/validate.go`, replace the `runValidate` function body (lines 26-67):

```go
func runValidate(cmd *cobra.Command, args []string) error {
	cfg := config.Get()
	if err := config.InitError(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	fmt.Println("Configuration valid!")
	fmt.Println()

	// Show unmatched behavior (first, most important setting)
	fmt.Printf("Unmatched command behavior: %s\n", cfg.Unmatched)

	// Show subshell settings
	fmt.Printf("Subshell allow all: %v\n", cfg.SubshellAllowAll)
	fmt.Println()

	// Show deny patterns
	fmt.Printf("Deny patterns: %d\n", len(cfg.DenyPatterns))
	for _, p := range cfg.DenyPatterns {
		fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
	}
	fmt.Println()

	// Show wrapper patterns
	fmt.Printf("Wrapper patterns: %d\n", len(cfg.WrapperPatterns))
	for _, p := range cfg.WrapperPatterns {
		fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
	}
	fmt.Println()

	// Show safe command patterns
	fmt.Printf("Safe command patterns: %d\n", len(cfg.SafeCommands))
	for _, p := range cfg.SafeCommands {
		fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())
	}
	fmt.Println()

	// Show rewrite rules
	fmt.Printf("Rewrite rules: %d\n", len(cfg.RewriteRules))
	for _, r := range cfg.RewriteRules {
		fmt.Printf("  [%s]  %q\t%s → %s\n", r.Type, r.Name, r.Regex.String(), r.Replace)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run "TestRunValidateShowsUnmatched|TestRunValidateUnmatched" -v`
Expected: All PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/validate.go cmd/validate_test.go
git commit -m "feat: show unmatched command behavior in validate output"
```

---

### Task 6: Update default config.toml with `[defaults]` section documentation

**Files:**
- Modify: `internal/config/config.toml`

- [ ] **Step 1: Add commented `[defaults]` section to config.toml**

In `internal/config/config.toml`, add a `[defaults]` section at the top, before the deny list section. Insert after the header comment (after line 13):

```toml
# ============================================================
# DEFAULTS - global behavior settings
# ============================================================

# [defaults]
# unmatched = "ask"  # "ask" (default), "passthrough", or "reject"
#   ask:         return "ask" to Claude Code (user gets prompted)
#   passthrough: return no output (Claude Code uses its own permission logic)
#   reject:      return "deny" to Claude Code (command blocked)

```

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.toml
git commit -m "docs: add [defaults] section documentation to config.toml"
```
