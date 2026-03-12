# Subshell `allow_all` Toggle Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `[subshell] allow_all` config option that skips the command-substitution rejection check, allowing `$(...)` and backtick commands to pass through to the normal deny/safe pipeline.

**Architecture:** New `SubshellAllowAll bool` field on `Config` struct, parsed from `[subshell]` TOML section using the existing `map[string]any` pattern. In the per-segment hook loop, a conditional guard skips `containsDangerousPattern()` when the toggle is `true`. Include merging uses last-value-wins semantics.

**Tech Stack:** Go, TOML (BurntSushi/toml), mvdan.cc/sh/v3/syntax (existing)

**Spec:** `docs/superpowers/specs/2026-03-12-subshell-allowlist-design.md`

---

## Chunk 1: Config parsing

### Task 1: Add `SubshellAllowAll` to Config struct and parse it

**Files:**
- Modify: `internal/config/config.go:21-28` (Config struct)
- Modify: `internal/config/config.go:220-293` (loadConfigWithIncludes)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test — config without `[subshell]` defaults to false**

```go
func TestLoadConfigSubshellDefaultsFalse(t *testing.T) {
	data := []byte(`
[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should default to false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellDefaultsFalse -v`
Expected: FAIL — `cfg.SubshellAllowAll` does not exist

- [ ] **Step 3: Add `SubshellAllowAll` field to Config struct**

In `internal/config/config.go`, add to the `Config` struct:

```go
type Config struct {
	// WrapperPatterns are safe prefixes that can wrap commands
	WrapperPatterns []patterns.Pattern
	// SafeCommands are patterns for allowed commands
	SafeCommands []patterns.Pattern
	// DenyPatterns are patterns that are always rejected (checked before approval)
	DenyPatterns []patterns.Pattern
	// SubshellAllowAll when true skips command substitution rejection
	SubshellAllowAll bool
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellDefaultsFalse -v`
Expected: PASS

- [ ] **Step 5: Write failing test — config with `[subshell] allow_all = true`**

```go
func TestLoadConfigSubshellAllowAllTrue(t *testing.T) {
	data := []byte(`
[subshell]
allow_all = true

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be true when allow_all = true")
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellAllowAllTrue -v`
Expected: FAIL — `SubshellAllowAll` is still `false` because parsing is not implemented

- [ ] **Step 7: Implement parsing in `loadConfigWithIncludes`**

In `internal/config/config.go`, add after the deny section parsing (after line 291) and before `return cfg, nil`:

```go
	// Parse subshell section
	if subshellSection, ok := raw["subshell"].(map[string]any); ok {
		if allowAll, ok := subshellSection["allow_all"].(bool); ok {
			cfg.SubshellAllowAll = allowAll
		}
	}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellAllowAllTrue -v`
Expected: PASS

- [ ] **Step 9: Write failing test — `allow_all = false` explicitly set**

```go
func TestLoadConfigSubshellAllowAllFalse(t *testing.T) {
	data := []byte(`
[subshell]
allow_all = false

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be false when allow_all = false")
	}
}
```

- [ ] **Step 10: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellAllowAllFalse -v`
Expected: PASS (already handled by the implementation)

- [ ] **Step 11: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add SubshellAllowAll config field with TOML parsing"
```

### Task 2: Include merging — last value wins

**Files:**
- Modify: `internal/config/config.go:228-265` (include merging)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test — included file sets `allow_all = true`**

```go
func TestLoadConfigSubshellAllowAllIncludeOverride(t *testing.T) {
	dir := t.TempDir()

	// Main config sets allow_all = false
	mainConfig := []byte(`
include = ["extra.toml"]

[subshell]
allow_all = false

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Included file sets allow_all = true
	extraConfig := []byte(`
[subshell]
allow_all = true
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Last value wins: main config (loaded after include) should override
	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be false — main config overrides included file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellAllowAllIncludeOverride -v`
Expected: FAIL — include merging does not handle `SubshellAllowAll`; the included file's `true` value persists

- [ ] **Step 3: Implement include merging for `SubshellAllowAll`**

In `internal/config/config.go`, in the include merging block (around line 261-264), add after the existing append lines:

```go
			// Merge included config
			cfg.WrapperPatterns = append(cfg.WrapperPatterns, includeCfg.WrapperPatterns...)
			cfg.SafeCommands = append(cfg.SafeCommands, includeCfg.SafeCommands...)
			cfg.DenyPatterns = append(cfg.DenyPatterns, includeCfg.DenyPatterns...)
			// SubshellAllowAll: unconditional assignment — last value wins.
			// If an included file omits [subshell], its zero value (false) will
			// overwrite a previous include's true. This is the safer default.
			cfg.SubshellAllowAll = includeCfg.SubshellAllowAll
```

Note: This works because includes are processed first, then the main config's `[subshell]` section is parsed afterward, overwriting the value. Unconditional assignment means that if a later include omits `[subshell]`, it resets to `false` — this is fail-secure.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellAllowAllIncludeOverride -v`
Expected: PASS

- [ ] **Step 5: Write test — included file sets true, main config omits section**

```go
func TestLoadConfigSubshellAllowAllFromInclude(t *testing.T) {
	dir := t.TempDir()

	// Main config does NOT have [subshell] section
	mainConfig := []byte(`
include = ["extra.toml"]

[[commands.simple]]
name = "main"
commands = ["echo"]
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Included file sets allow_all = true
	extraConfig := []byte(`
[subshell]
allow_all = true
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}
	if !cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be true — inherited from included file")
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellAllowAllFromInclude -v`
Expected: PASS

- [ ] **Step 7: Write test — invalid type for `allow_all` silently defaults to false**

```go
func TestLoadConfigSubshellAllowAllInvalidType(t *testing.T) {
	data := []byte(`
[subshell]
allow_all = "yes"

[[commands.simple]]
name = "test"
commands = ["echo"]
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.SubshellAllowAll {
		t.Error("SubshellAllowAll should be false when allow_all has invalid type")
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -run TestLoadConfigSubshellAllowAllInvalidType -v`
Expected: PASS (type assertion `.(bool)` fails silently, field stays false)

- [ ] **Step 9: Run all config tests**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/config/ -v`
Expected: All tests PASS

- [ ] **Step 10: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add include merging for SubshellAllowAll (last value wins)"
```

## Chunk 2: Hook logic

### Task 3: Skip `containsDangerousPattern` when `SubshellAllowAll` is true

**Files:**
- Modify: `internal/hook/hook.go:233-247` (per-segment dangerous pattern check)
- Test: `internal/hook/hook_test.go`

- [ ] **Step 1: Write failing test — `$(...)` rejected when `allow_all` is false (existing behavior)**

This test confirms the existing behavior still works. It should pass immediately.

```go
func TestCommandSubstitutionRejectedByDefault(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[[commands.simple]]
name = "git"
commands = ["git"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "git commit -m \"$(cat <<'EOF'\nfix bug\nEOF\n)\""}
	}`

	result := ProcessWithResult(strings.NewReader(input))
	if result.Approved {
		t.Error("Command with $() should be rejected when allow_all is false")
	}

	entry := readLastAuditEntry(t, logPath)
	if entry.Segments[0].Rejection == nil {
		t.Fatal("Expected rejection for command substitution")
	}
	if entry.Segments[0].Rejection.Code != audit.CodeCommandSubstitution {
		t.Errorf("Rejection.Code = %q, want %q", entry.Segments[0].Rejection.Code, audit.CodeCommandSubstitution)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/hook/ -run TestCommandSubstitutionRejectedByDefault -v`
Expected: PASS

- [ ] **Step 3: Write failing test — `$(...)` approved when `allow_all = true`**

```go
func TestCommandSubstitutionAllowedWhenAllowAll(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[subshell]
allow_all = true

[[commands.subcommand]]
command = "git"
subcommands = ["commit"]
flags = ["-m <arg>"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "git commit -m \"$(cat <<'EOF'\nfix bug\nEOF\n)\""}
	}`

	result := ProcessWithResult(strings.NewReader(input))
	if !result.Approved {
		t.Errorf("Command with $() should be approved when allow_all is true, got output: %s", result.Output)
	}

	entry := readLastAuditEntry(t, logPath)
	if !entry.Approved {
		t.Error("Audit entry should show approved")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/hook/ -run TestCommandSubstitutionAllowedWhenAllowAll -v`
Expected: FAIL — `containsDangerousPattern` still rejects `$(...)`

- [ ] **Step 5: Add conditional guard in `ProcessWithResult`**

In `internal/hook/hook.go`, modify the dangerous pattern check block (lines 233-247):

```go
		// Check for dangerous patterns (command substitution) in this segment
		if !cfg.SubshellAllowAll && containsDangerousPattern(segment) {
			logger.Debug("rejected dangerous pattern in segment", "segment", segment)
			overallApproved = false
			auditSegments = append(auditSegments, audit.Segment{
				Command:  segment,
				Approved: false,
				Wrappers: wrappers,
				Rejection: &audit.Rejection{
					Code:    audit.CodeCommandSubstitution,
					Pattern: "$(...)",
				},
			})
			continue
		}
```

The only change is adding `!cfg.SubshellAllowAll &&` to the condition.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/hook/ -run TestCommandSubstitutionAllowedWhenAllowAll -v`
Expected: PASS

- [ ] **Step 7: Write test — backticks approved when `allow_all = true`**

```go
func TestBackticksAllowedWhenAllowAll(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[subshell]
allow_all = true

[[commands.simple]]
name = "basic"
commands = ["echo"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "echo ` + "`date`" + `"}
	}`

	result := ProcessWithResult(strings.NewReader(input))
	if !result.Approved {
		t.Errorf("Command with backticks should be approved when allow_all is true, got output: %s", result.Output)
	}

	entry := readLastAuditEntry(t, logPath)
	if !entry.Approved {
		t.Error("Audit entry should show approved")
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/hook/ -run TestBackticksAllowedWhenAllowAll -v`
Expected: PASS (already handled by the implementation)

- [ ] **Step 9: Write test — deny patterns still reject when `allow_all = true`**

```go
func TestDenyStillRejectsWhenAllowAll(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[subshell]
allow_all = true

[[deny.simple]]
name = "privilege escalation"
commands = ["sudo"]

[[commands.subcommand]]
command = "git"
subcommands = ["commit"]
flags = ["-m <arg>"]
`)
	defer cleanupConfig()

	logPath, cleanupAudit := setupTestAudit(t)
	defer cleanupAudit()

	input := `{
		"session_id": "sess-1",
		"tool_use_id": "tool-1",
		"cwd": "/home",
		"tool_name": "Bash",
		"tool_input": {"command": "sudo git commit -m \"$(cat <<'EOF'\nfix\nEOF\n)\""}
	}`

	result := ProcessWithResult(strings.NewReader(input))
	if result.Approved {
		t.Error("Denied command should still be rejected even with allow_all = true")
	}

	entry := readLastAuditEntry(t, logPath)
	if entry.Approved {
		t.Error("Audit entry should show rejected")
	}
}
```

- [ ] **Step 10: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/hook/ -run TestDenyStillRejectsWhenAllowAll -v`
Expected: PASS (deny check is unchanged)

- [ ] **Step 11: Write test — no-match still rejects when `allow_all = true`**

```go
func TestNoMatchStillRejectsWhenAllowAll(t *testing.T) {
	cleanupConfig := setupTestConfig(t, `
[subshell]
allow_all = true

[[commands.simple]]
name = "basic"
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
		"tool_input": {"command": "unknown-cmd $(echo hi)"}
	}`

	result := ProcessWithResult(strings.NewReader(input))
	if result.Approved {
		t.Error("Unknown command should still be rejected even with allow_all = true")
	}

	entry := readLastAuditEntry(t, logPath)
	if entry.Segments[0].Rejection == nil {
		t.Fatal("Expected rejection")
	}
	if entry.Segments[0].Rejection.Code != audit.CodeNoMatch {
		t.Errorf("Rejection.Code = %q, want %q", entry.Segments[0].Rejection.Code, audit.CodeNoMatch)
	}
}
```

- [ ] **Step 12: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/hook/ -run TestNoMatchStillRejectsWhenAllowAll -v`
Expected: PASS

- [ ] **Step 13: Run all hook tests**

Run: `cd /Users/dgerlanc/code/mmi && go test ./internal/hook/ -v`
Expected: All tests PASS

- [ ] **Step 14: Commit**

```bash
git add internal/hook/hook.go internal/hook/hook_test.go
git commit -m "feat: skip command substitution check when SubshellAllowAll is true"
```

## Chunk 3: Validate output and default config

### Task 4: Show `SubshellAllowAll` in `mmi validate` output

**Files:**
- Modify: `cmd/validate.go:26-56` (runValidate)
- Test: `cmd/validate_test.go`

- [ ] **Step 1: Write failing test — validate shows subshell allow_all status**

```go
func TestRunValidateShowsSubshellAllowAll(t *testing.T) {
	resetGlobalState()

	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	testConfig := `
[subshell]
allow_all = true

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

	if !strings.Contains(output, "Subshell allow all: true") {
		t.Errorf("expected 'Subshell allow all: true' in output, got:\n%s", output)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/dgerlanc/code/mmi && go test ./cmd/ -run TestRunValidateShowsSubshellAllowAll -v`
Expected: FAIL — validate output does not include subshell info

- [ ] **Step 3: Add subshell status to validate output**

In `cmd/validate.go`, add after the "Configuration valid!" line (line 32):

```go
	fmt.Println("Configuration valid!")
	fmt.Println()

	// Show subshell settings
	fmt.Printf("Subshell allow all: %v\n", cfg.SubshellAllowAll)
	fmt.Println()
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./cmd/ -run TestRunValidateShowsSubshellAllowAll -v`
Expected: PASS

- [ ] **Step 5: Write test — validate shows false when not set**

```go
func TestRunValidateShowsSubshellAllowAllFalse(t *testing.T) {
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

	if !strings.Contains(output, "Subshell allow all: false") {
		t.Errorf("expected 'Subshell allow all: false' in output, got:\n%s", output)
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /Users/dgerlanc/code/mmi && go test ./cmd/ -run TestRunValidateShowsSubshellAllowAllFalse -v`
Expected: PASS

- [ ] **Step 7: Run all cmd tests**

Run: `cd /Users/dgerlanc/code/mmi && go test ./cmd/ -v`
Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/validate.go cmd/validate_test.go
git commit -m "feat: show SubshellAllowAll status in mmi validate output"
```

### Task 5: Add commented-out `[subshell]` section to default config

**Files:**
- Modify: `internal/config/config.toml`

- [ ] **Step 1: Add commented-out section to default config**

Add at the end of `internal/config/config.toml`:

```toml
# ============================================================
# SUBSHELL - control command substitution ($() and backtick) handling
# ============================================================

# [subshell]
# allow_all = false  # set to true to permit all command substitution
```

- [ ] **Step 2: Run all tests to verify nothing is broken**

Run: `cd /Users/dgerlanc/code/mmi && go test ./... -v`
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.toml
git commit -m "docs: add commented-out [subshell] section to default config"
```

## Chunk 4: Documentation updates

### Task 6: Update SPEC.md and PATTERNS.md

**Files:**
- Modify: `docs/SPEC.md`
- Modify: `docs/PATTERNS.md` (if relevant)

- [ ] **Step 1: Update SPEC.md section 4.3**

In `docs/SPEC.md`, find section 4.3 "Dangerous Pattern Detection" and update:

Replace:
```
Command substitution is always rejected:
```

With:
```
Command substitution is rejected by default:
```

Add after the "Per-segment checking" paragraph:

```markdown
**Subshell configuration**: The `[subshell]` config section controls command substitution handling:

```toml
[subshell]
allow_all = false  # default; set true to permit all $() and backticks
```

When `allow_all = true`, the dangerous pattern check is skipped entirely. Commands still pass through the deny → wrapper → safe pipeline. This is useful for workflows where Claude Code generates commands like `git commit -m "$(cat <<'EOF' ... EOF)"`.
```

- [ ] **Step 2: Update SPEC.md section 9.1**

Find section 9.1 which states "Command substitution always rejected" and update to:

```
Command substitution rejected by default (configurable via `[subshell] allow_all`)
```

- [ ] **Step 3: Update SPEC.md configuration reference**

Find the configuration section that lists available config sections and add `[subshell]` with its fields.

- [ ] **Step 4: Commit**

```bash
git add docs/SPEC.md docs/PATTERNS.md
git commit -m "docs: update SPEC.md for subshell allow_all configuration"
```

### Task 7: Run full test suite

- [ ] **Step 1: Run all tests**

Run: `cd /Users/dgerlanc/code/mmi && go test ./... -v`
Expected: All tests PASS

- [ ] **Step 2: Run benchmarks to check for regressions**

Run: `cd /Users/dgerlanc/code/mmi && go test -bench=. -benchmem ./...`
Expected: No significant regressions
