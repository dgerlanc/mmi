# Command Rewrites Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add command rewriting that rejects commands matching rewrite rules and suggests corrected alternatives (e.g., `python` → `uv run python`).

**Architecture:** New `RewriteRule` type in `patterns` package, `parseRewriteSection` in `config` package, `CheckRewrite` function and pipeline integration in `hook` package. All inline — no new packages.

**Tech Stack:** Go, TOML config, regex matching

**Spec:** `docs/superpowers/specs/2026-03-23-command-rewrites-design.md`

---

### File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/patterns/patterns.go` | Add `RewriteRule` struct |
| Modify | `internal/patterns/patterns_test.go` | Tests for `RewriteRule` |
| Modify | `internal/audit/audit.go:16-21` | Add `CodeRewrite` constant |
| Modify | `internal/config/config.go:21-30` | Add `RewriteRules` field to `Config` |
| Modify | `internal/config/config.go:222-307` | Parse `[[rewrites.*]]` section |
| Modify | `internal/config/config_test.go` | Tests for rewrite config parsing |
| Modify | `internal/hook/hook.go:322-343` | Add `RewriteResult` type and `CheckRewrite` function |
| Modify | `internal/hook/hook.go:224-319` | Integrate rewrite check into pipeline |
| Modify | `internal/hook/hook_test.go` | Tests for `CheckRewrite` and pipeline integration |
| Modify | `cmd/validate.go:26-60` | Display rewrite rules |
| Modify | `main_test.go:19-95` | Add rewrite rules to `testConfig`, add integration tests |

---

### Task 1: Add `RewriteRule` type to patterns package

**Files:**
- Modify: `internal/patterns/patterns.go:10-16`
- Test: `internal/patterns/patterns_test.go`

- [ ] **Step 1: Write test for RewriteRule struct**

In `internal/patterns/patterns_test.go`, add:

```go
func TestRewriteRuleFields(t *testing.T) {
	re := regexp.MustCompile(`^python\b`)
	rule := patterns.RewriteRule{
		Regex:   re,
		Name:    "use uv",
		Type:    "simple",
		Pattern: `^python\b`,
		Replace: "uv run python",
	}
	if rule.Regex != re {
		t.Error("Regex field not set")
	}
	if rule.Name != "use uv" {
		t.Errorf("Name = %q, want %q", rule.Name, "use uv")
	}
	if rule.Replace != "uv run python" {
		t.Errorf("Replace = %q, want %q", rule.Replace, "uv run python")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/patterns/ -run TestRewriteRuleFields -v`
Expected: FAIL — `RewriteRule` not defined

- [ ] **Step 3: Add RewriteRule struct**

In `internal/patterns/patterns.go`, after the `Pattern` struct (line 16), add:

```go
// RewriteRule holds a compiled match pattern and its replacement string.
type RewriteRule struct {
	Regex   *regexp.Regexp
	Name    string
	Type    string // "simple" or "regex"
	Pattern string // original pattern string
	Replace string // replacement string
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/patterns/ -run TestRewriteRuleFields -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/patterns/patterns.go internal/patterns/patterns_test.go
git commit -m "feat: add RewriteRule type to patterns package"
```

---

### Task 2: Add `CodeRewrite` audit constant

**Files:**
- Modify: `internal/audit/audit.go:16-21`

- [ ] **Step 1: Add CodeRewrite constant**

In `internal/audit/audit.go`, add to the rejection codes const block (line 20):

```go
CodeRewrite = "REWRITE"
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/audit/`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/audit/audit.go
git commit -m "feat: add REWRITE rejection code to audit package"
```

---

### Task 3: Add `RewriteRules` to Config and parse `[[rewrites.*]]`

**Files:**
- Modify: `internal/config/config.go:21-30` (Config struct)
- Modify: `internal/config/config.go:222-307` (loadConfigWithIncludes)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write tests for rewrite config parsing**

In `internal/config/config_test.go`, add:

```go
func TestLoadConfigRewritesSimple(t *testing.T) {
	data := []byte(`
[[rewrites.simple]]
name = "use uv"
match = ["python", "python3"]
replace = "uv run python"
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.RewriteRules) != 2 {
		t.Fatalf("expected 2 rewrite rules, got %d", len(cfg.RewriteRules))
	}
	if cfg.RewriteRules[0].Name != "use uv" {
		t.Errorf("Name = %q, want %q", cfg.RewriteRules[0].Name, "use uv")
	}
	if cfg.RewriteRules[0].Replace != "uv run python" {
		t.Errorf("Replace = %q, want %q", cfg.RewriteRules[0].Replace, "uv run python")
	}
	if cfg.RewriteRules[0].Type != "simple" {
		t.Errorf("Type = %q, want %q", cfg.RewriteRules[0].Type, "simple")
	}
}

func TestLoadConfigRewritesRegex(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "use uv for pip"
pattern = '^pip3?\b'
replace = "uv pip"
`)
	cfg, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.RewriteRules) != 1 {
		t.Fatalf("expected 1 rewrite rule, got %d", len(cfg.RewriteRules))
	}
	if cfg.RewriteRules[0].Name != "use uv for pip" {
		t.Errorf("Name = %q, want %q", cfg.RewriteRules[0].Name, "use uv for pip")
	}
	if cfg.RewriteRules[0].Type != "regex" {
		t.Errorf("Type = %q, want %q", cfg.RewriteRules[0].Type, "regex")
	}
}

func TestLoadConfigRewritesSimpleMissingMatch(t *testing.T) {
	data := []byte(`
[[rewrites.simple]]
name = "use uv"
replace = "uv run python"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing match field")
	}
	if !strings.Contains(err.Error(), "rewrites.simple[0]") {
		t.Errorf("error should reference rewrites.simple[0], got: %v", err)
	}
	if !strings.Contains(err.Error(), "\"match\" field is required") {
		t.Errorf("error should mention match field, got: %v", err)
	}
}

func TestLoadConfigRewritesSimpleMissingReplace(t *testing.T) {
	data := []byte(`
[[rewrites.simple]]
name = "use uv"
match = ["python"]
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing replace field")
	}
	if !strings.Contains(err.Error(), "\"replace\" field is required") {
		t.Errorf("error should mention replace field, got: %v", err)
	}
}

func TestLoadConfigRewritesRegexMissingPattern(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "use uv"
replace = "uv pip"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing pattern field")
	}
	if !strings.Contains(err.Error(), "rewrites.regex[0]") {
		t.Errorf("error should reference rewrites.regex[0], got: %v", err)
	}
}

func TestLoadConfigRewritesRegexMissingReplace(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "use uv"
pattern = '^pip3?\b'
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for missing replace field")
	}
}

func TestLoadConfigRewritesRegexInvalidPattern(t *testing.T) {
	data := []byte(`
[[rewrites.regex]]
name = "bad"
pattern = '[invalid'
replace = "foo"
`)
	_, err := LoadConfig(data)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestLoadConfigRewritesMergeIncludes(t *testing.T) {
	dir := t.TempDir()

	mainConfig := []byte(`
include = ["extra.toml"]

[[rewrites.simple]]
name = "main rewrite"
match = ["python"]
replace = "uv run python"
`)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), mainConfig, 0644); err != nil {
		t.Fatal(err)
	}

	extraConfig := []byte(`
[[rewrites.simple]]
name = "extra rewrite"
match = ["pip"]
replace = "uv pip"
`)
	if err := os.WriteFile(filepath.Join(dir, "extra.toml"), extraConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigWithDir(mainConfig, dir)
	if err != nil {
		t.Fatalf("LoadConfigWithDir failed: %v", err)
	}

	if len(cfg.RewriteRules) != 2 {
		t.Errorf("expected 2 rewrite rules after merge, got %d", len(cfg.RewriteRules))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoadConfigRewrites -v`
Expected: FAIL — `RewriteRules` field not on `Config`

- [ ] **Step 3: Add RewriteRules field to Config**

In `internal/config/config.go`, add to the `Config` struct (after line 29):

```go
	// RewriteRules are patterns that trigger command rewrite suggestions
	RewriteRules []patterns.RewriteRule
```

- [ ] **Step 4: Add parseRewriteSection function**

In `internal/config/config.go`, add after `parseDenySection` (after line 361):

```go
// parseRewriteSection parses the rewrites section of the config.
// Rewrite rules use simple and regex subsections.
func parseRewriteSection(sectionData map[string]any) ([]patterns.RewriteRule, error) {
	var result []patterns.RewriteRule

	for sectionType, value := range sectionData {
		switch sectionType {
		case "simple":
			entries := toMapSlice(value)
			for i, entry := range entries {
				name, _ := entry["name"].(string)
				cmds := toStringSlice(entry["match"])
				replace, _ := entry["replace"].(string)
				if len(cmds) == 0 {
					if name != "" {
						return nil, fmt.Errorf("rewrites.simple[%d] %q: \"match\" field is required and must not be empty", i, name)
					}
					return nil, fmt.Errorf("rewrites.simple[%d]: \"match\" field is required and must not be empty", i)
				}
				if replace == "" {
					if name != "" {
						return nil, fmt.Errorf("rewrites.simple[%d] %q: \"replace\" field is required and must not be empty", i, name)
					}
					return nil, fmt.Errorf("rewrites.simple[%d]: \"replace\" field is required and must not be empty", i)
				}
				for _, cmd := range cmds {
					pattern := patterns.BuildSimplePattern(cmd)
					re, err := regexp.Compile(pattern)
					if err != nil {
						return nil, fmt.Errorf("invalid rewrite pattern for command %q: %w", cmd, err)
					}
					result = append(result, patterns.RewriteRule{
						Regex:   re,
						Name:    name,
						Type:    "simple",
						Pattern: pattern,
						Replace: replace,
					})
				}
			}

		case "regex":
			entries := toMapSlice(value)
			for i, entry := range entries {
				pattern, _ := entry["pattern"].(string)
				name, _ := entry["name"].(string)
				replace, _ := entry["replace"].(string)
				if pattern == "" {
					if name != "" {
						return nil, fmt.Errorf("rewrites.regex[%d] %q: \"pattern\" field is required and must not be empty", i, name)
					}
					return nil, fmt.Errorf("rewrites.regex[%d]: \"pattern\" field is required and must not be empty", i)
				}
				if replace == "" {
					if name != "" {
						return nil, fmt.Errorf("rewrites.regex[%d] %q: \"replace\" field is required and must not be empty", i, name)
					}
					return nil, fmt.Errorf("rewrites.regex[%d]: \"replace\" field is required and must not be empty", i)
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid rewrite regex pattern %q: %w", pattern, err)
				}
				result = append(result, patterns.RewriteRule{
					Regex:   re,
					Name:    name,
					Type:    "regex",
					Pattern: pattern,
					Replace: replace,
				})
			}
		}
	}

	return result, nil
}
```

- [ ] **Step 5: Add rewrites parsing to loadConfigWithIncludes**

In `internal/config/config.go`, in `loadConfigWithIncludes`:

After the include merge block (after line 270, `cfg.SubshellAllowAll = includeCfg.SubshellAllowAll`), add:

```go
			cfg.RewriteRules = append(cfg.RewriteRules, includeCfg.RewriteRules...)
```

After the subshell section parsing (after line 304, before `return cfg, nil`), add:

```go
	// Parse rewrites section
	if rewritesSection, ok := raw["rewrites"].(map[string]any); ok {
		rewrites, err := parseRewriteSection(rewritesSection)
		if err != nil {
			return nil, fmt.Errorf("failed to parse rewrites: %w", err)
		}
		cfg.RewriteRules = append(cfg.RewriteRules, rewrites...)
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestLoadConfigRewrites -v`
Expected: All PASS

- [ ] **Step 7: Run full config test suite**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add rewrite rules config parsing"
```

---

### Task 4: Add `CheckRewrite` function to hook package

**Files:**
- Modify: `internal/hook/hook.go` (after `CheckDeny`, ~line 364)
- Test: `internal/hook/hook_test.go`

- [ ] **Step 1: Write tests for CheckRewrite**

In `internal/hook/hook_test.go`, add:

```go
func TestCheckRewrite(t *testing.T) {
	simpleRules := []patterns.RewriteRule{
		{
			Regex:   regexp.MustCompile(`^python\b`),
			Name:    "use uv",
			Type:    "simple",
			Pattern: `^python\b`,
			Replace: "uv run python",
		},
		{
			Regex:   regexp.MustCompile(`^python3\b`),
			Name:    "use uv",
			Type:    "simple",
			Pattern: `^python3\b`,
			Replace: "uv run python",
		},
	}

	regexRules := []patterns.RewriteRule{
		{
			Regex:   regexp.MustCompile(`^pip3?\b`),
			Name:    "use uv for pip",
			Type:    "regex",
			Pattern: `^pip3?\b`,
			Replace: "uv pip",
		},
	}

	tests := []struct {
		name        string
		coreCmd     string
		rules       []patterns.RewriteRule
		wantMatched bool
		wantReplace string
	}{
		{
			name:        "simple match preserves args",
			coreCmd:     "python script.py --verbose",
			rules:       simpleRules,
			wantMatched: true,
			wantReplace: "uv run python script.py --verbose",
		},
		{
			name:        "simple match python3",
			coreCmd:     "python3 script.py",
			rules:       simpleRules,
			wantMatched: true,
			wantReplace: "uv run python script.py",
		},
		{
			name:        "simple match bare command",
			coreCmd:     "python",
			rules:       simpleRules,
			wantMatched: true,
			wantReplace: "uv run python",
		},
		{
			name:        "simple no match",
			coreCmd:     "ruby script.rb",
			rules:       simpleRules,
			wantMatched: false,
		},
		{
			name:        "regex match pip",
			coreCmd:     "pip install requests",
			rules:       regexRules,
			wantMatched: true,
			wantReplace: "uv pip install requests",
		},
		{
			name:        "regex match pip3",
			coreCmd:     "pip3 install requests",
			rules:       regexRules,
			wantMatched: true,
			wantReplace: "uv pip install requests",
		},
		{
			name:        "regex no match",
			coreCmd:     "npm install",
			rules:       regexRules,
			wantMatched: false,
		},
		{
			name:        "empty rules",
			coreCmd:     "python script.py",
			rules:       nil,
			wantMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckRewrite(tt.coreCmd, tt.rules)
			if result.Matched != tt.wantMatched {
				t.Errorf("Matched = %v, want %v", result.Matched, tt.wantMatched)
			}
			if tt.wantMatched && result.Replacement != tt.wantReplace {
				t.Errorf("Replacement = %q, want %q", result.Replacement, tt.wantReplace)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hook/ -run TestCheckRewrite -v`
Expected: FAIL — `CheckRewrite` not defined

- [ ] **Step 3: Implement CheckRewrite**

In `internal/hook/hook.go`, after `CheckDeny` (after line 364), add:

```go
// RewriteResult contains detailed information about a rewrite rule match.
type RewriteResult struct {
	Matched     bool
	Name        string
	Pattern     string
	Replacement string // the fully rewritten core command
}

// CheckRewrite checks if a command matches a rewrite rule and returns the suggested replacement.
// For simple rules, the matched prefix is replaced and remaining arguments are preserved.
// For regex rules, Regexp.ReplaceAllString is used for full control.
func CheckRewrite(coreCmd string, rules []patterns.RewriteRule) RewriteResult {
	for _, r := range rules {
		loc := r.Regex.FindStringIndex(coreCmd)
		if loc == nil {
			continue
		}
		var replacement string
		if r.Type == "simple" {
			// Replace the matched prefix, preserve the rest
			replacement = r.Replace + coreCmd[loc[1]:]
		} else {
			// Regex: use ReplaceAllString for capture group support
			replacement = r.Regex.ReplaceAllString(coreCmd, r.Replace)
		}
		return RewriteResult{
			Matched:     true,
			Name:        r.Name,
			Pattern:     r.Pattern,
			Replacement: replacement,
		}
	}
	return RewriteResult{Matched: false}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/hook/ -run TestCheckRewrite -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hook/hook.go internal/hook/hook_test.go
git commit -m "feat: add CheckRewrite function"
```

---

### Task 5: Integrate rewrite check into the hook pipeline

**Files:**
- Modify: `internal/hook/hook.go:224-319` (ProcessWithResult segment loop and post-loop)
- Test: `internal/hook/hook_test.go`

- [ ] **Step 1: Write tests for pipeline integration**

In `internal/hook/hook_test.go`, add:

```go
func TestProcessWithResultRewrite(t *testing.T) {
	// Set up config with a safe python command AND a rewrite rule
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	cfgData := `
[[commands.simple]]
name = "python"
commands = ["python", "python3"]

[[commands.subcommand]]
command = "git"
subcommands = ["status"]

[[rewrites.simple]]
name = "use uv"
match = ["python", "python3"]
replace = "uv run python"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(cfgData), 0644); err != nil {
		t.Fatal(err)
	}
	config.Reset()
	config.Init()
	defer config.Reset()

	tests := []struct {
		name       string
		command    string
		wantAsk    bool
		wantReason string
	}{
		{
			name:       "safe command with rewrite gets rewritten",
			command:    "python3 script.py",
			wantAsk:    true,
			wantReason: `rewrite: use "uv run python script.py" instead of "python3 script.py"`,
		},
		{
			name:       "no rewrite match gets approved",
			command:    "git status",
			wantAsk:    false,
			wantReason: "",
		},
		{
			name:       "chain with rewrite",
			command:    "git status && python3 script.py",
			wantAsk:    true,
			wantReason: `rewrite: use "uv run python script.py" instead of "python3 script.py"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"tool_name":"Bash","tool_input":{"command":"` + tt.command + `"}}`
			result := ProcessWithResult(strings.NewReader(input))

			if tt.wantAsk {
				if result.Approved {
					t.Error("expected rejection, got approval")
				}
				// Parse the output JSON to check reason
				var output Output
				if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
					t.Fatalf("failed to parse output: %v", err)
				}
				if output.HookSpecificOutput.PermissionDecision != DecisionAsk {
					t.Errorf("decision = %q, want %q", output.HookSpecificOutput.PermissionDecision, DecisionAsk)
				}
				if output.HookSpecificOutput.PermissionDecisionReason != tt.wantReason {
					t.Errorf("reason = %q, want %q", output.HookSpecificOutput.PermissionDecisionReason, tt.wantReason)
				}
			} else {
				if !result.Approved {
					t.Errorf("expected approval, got rejection: %s", result.Output)
				}
			}
		})
	}
}

func TestProcessWithResultRewriteSkipsDeny(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")

	cfgData := `
[[deny.simple]]
name = "no sudo"
commands = ["sudo"]

[[rewrites.simple]]
name = "rewrite sudo"
match = ["sudo"]
replace = "doas"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(cfgData), 0644); err != nil {
		t.Fatal(err)
	}
	config.Reset()
	config.Init()
	defer config.Reset()

	input := `{"tool_name":"Bash","tool_input":{"command":"sudo apt install foo"}}`
	result := ProcessWithResult(strings.NewReader(input))

	// Should be denied, not rewritten
	var output Output
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if output.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("decision = %q, want %q", output.HookSpecificOutput.PermissionDecision, "deny")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/hook/ -run TestProcessWithResultRewrite -v`
Expected: FAIL — pipeline doesn't check rewrites yet

- [ ] **Step 3: Integrate rewrite check into ProcessWithResult**

In `internal/hook/hook.go`, modify the `ProcessWithResult` function. The key changes:

**Add tracking variables** (after line 222, `hasDenyMatch := false`):

```go
	hasRewrite := false
	var rewriteSuggestions []string
```

**Replace the safe-check-and-approve block** (lines 268-300). The current code has a `continue` on `!safeResult.Matched`, then appends an approved segment. Replace with logic that checks rewrites regardless of safe match:

After the deny check (line 266), replace lines 268-300 with:

```go
		// Check safe patterns
		safeResult := CheckSafe(coreCmd, cfg.SafeCommands)

		// Check rewrite rules (regardless of safe match)
		rewriteResult := CheckRewrite(coreCmd, cfg.RewriteRules)
		if rewriteResult.Matched {
			logger.Debug("rewrite matched", "command", coreCmd, "replacement", rewriteResult.Replacement)
			overallApproved = false
			hasRewrite = true
			rewriteSuggestions = append(rewriteSuggestions, fmt.Sprintf("use %q instead of %q", rewriteResult.Replacement, coreCmd))
			auditSegments = append(auditSegments, audit.Segment{
				Command:  segment,
				Approved: false,
				Wrappers: wrappers,
				Rejection: &audit.Rejection{
					Code:    audit.CodeRewrite,
					Name:    rewriteResult.Name,
					Pattern: rewriteResult.Pattern,
					Detail:  rewriteResult.Replacement,
				},
			})
			continue
		}

		if !safeResult.Matched {
			logger.Debug("rejected unsafe command", "command", coreCmd)
			overallApproved = false
			auditSegments = append(auditSegments, audit.Segment{
				Command:   segment,
				Approved:  false,
				Wrappers:  wrappers,
				Rejection: &audit.Rejection{Code: audit.CodeNoMatch},
			})
			continue
		}

		logger.Debug("matched pattern", "command", coreCmd, "pattern", safeResult.Name)

		// Approved segment
		auditSegments = append(auditSegments, audit.Segment{
			Command:  segment,
			Approved: true,
			Wrappers: wrappers,
			Match: &audit.Match{
				Type:    safeResult.Type,
				Name:    safeResult.Name,
				Pattern: safeResult.Pattern,
			},
		})

		if len(wrappers) > 0 {
			reasons = append(reasons, strings.Join(wrappers, "+")+" + "+safeResult.Name)
		} else {
			reasons = append(reasons, safeResult.Name)
		}
```

**Add rewrite handling to post-loop logic** (lines 303-313). After the existing `if !overallApproved` block, but restructure it to handle rewrites. Replace lines 305-313 with:

```go
	if !overallApproved {
		var output string
		if hasDenyMatch {
			output = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"command matches deny list"}}`
		} else if hasRewrite {
			reason := "rewrite: " + strings.Join(rewriteSuggestions, "; ")
			output = FormatAsk(reason)
		} else {
			output = FormatAsk("command not in allow list")
		}
		logAudit(cmd, false, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd, rawInput, output)
		return Result{Command: cmd, Approved: false, Output: output}
	}
```

Add `"fmt"` to imports if not already present.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hook/ -run TestProcessWithResultRewrite -v`
Expected: All PASS

- [ ] **Step 5: Run full hook test suite**

Run: `go test ./internal/hook/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/hook/hook.go internal/hook/hook_test.go
git commit -m "feat: integrate rewrite check into hook pipeline"
```

---

### Task 6: Add rewrite rules to validate command output

**Files:**
- Modify: `cmd/validate.go:26-60`

- [ ] **Step 1: Add rewrite rules display**

In `cmd/validate.go`, after the safe command patterns block (after line 57, `fmt.Printf("  - %s: %s\n", p.Name, p.Regex.String())`), add:

```go
	fmt.Println()

	// Show rewrite rules
	fmt.Printf("Rewrite rules: %d\n", len(cfg.RewriteRules))
	for _, r := range cfg.RewriteRules {
		fmt.Printf("  [%s]  %q\t%s → %s\n", r.Type, r.Name, r.Regex.String(), r.Replace)
	}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/...`
Expected: Success (note: `cmd` is a subpackage of the root, so build from project root)

Actually, since `cmd/` uses internal `config` package, build the whole binary:

Run: `go build .`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add cmd/validate.go
git commit -m "feat: display rewrite rules in validate command"
```

---

### Task 7: Integration tests

**Files:**
- Modify: `main_test.go:19-95` (testConfig) and add new tests

- [ ] **Step 1: Add rewrite rules to testConfig**

In `main_test.go`, add the following to the `testConfig` string (before the closing backtick on line 95):

```toml

# Rewrites
[[rewrites.simple]]
name = "use uv for python"
match = ["python", "python3"]
replace = "uv run python"

[[rewrites.regex]]
name = "use uv for pip"
pattern = '^pip3?\b'
replace = "uv pip"
```

- [ ] **Step 2: Write integration tests**

In `main_test.go`, add:

```go
func TestProcessRewriteIntegration(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectApproved bool
		expectReason   string
	}{
		{
			"rewrite python3",
			`{"tool_name":"Bash","tool_input":{"command":"python3 script.py"}}`,
			false,
			`rewrite: use "uv run python script.py" instead of "python3 script.py"`,
		},
		{
			"rewrite pip",
			`{"tool_name":"Bash","tool_input":{"command":"pip install requests"}}`,
			false,
			`rewrite: use "uv pip install requests" instead of "pip install requests"`,
		},
		{
			"rewrite pip3",
			`{"tool_name":"Bash","tool_input":{"command":"pip3 install requests"}}`,
			false,
			`rewrite: use "uv pip install requests" instead of "pip3 install requests"`,
		},
		{
			"chain with rewrite",
			`{"tool_name":"Bash","tool_input":{"command":"git status && python3 script.py"}}`,
			false,
			`rewrite: use "uv run python script.py" instead of "python3 script.py"`,
		},
		{
			"no rewrite for safe command",
			`{"tool_name":"Bash","tool_input":{"command":"git status"}}`,
			true,
			"git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hook.ProcessWithResult(strings.NewReader(tt.input))
			if result.Approved != tt.expectApproved {
				t.Errorf("Approved = %v, want %v (output: %s)", result.Approved, tt.expectApproved, result.Output)
			}
			if tt.expectApproved {
				if result.Reason != tt.expectReason {
					t.Errorf("Reason = %q, want %q", result.Reason, tt.expectReason)
				}
			} else {
				var output hook.Output
				if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
					t.Fatalf("failed to parse output: %v", err)
				}
				if output.HookSpecificOutput.PermissionDecisionReason != tt.expectReason {
					t.Errorf("reason = %q, want %q", output.HookSpecificOutput.PermissionDecisionReason, tt.expectReason)
				}
			}
		})
	}
}
```

- [ ] **Step 3: Run integration tests**

Run: `go test -run TestProcessRewriteIntegration -v`
Expected: All PASS

- [ ] **Step 4: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add main_test.go
git commit -m "test: add integration tests for command rewrites"
```

---

### Task 8: Verify existing tests still pass

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `go test ./...`
Expected: All PASS — no regressions

- [ ] **Step 2: Run with race detector**

Run: `go test -race ./...`
Expected: All PASS, no races

- [ ] **Step 3: Run vet and build**

Run: `go vet ./... && go build .`
Expected: No warnings, successful build
