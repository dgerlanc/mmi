# Context-Aware Path Checking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make mmi approval decisions directory-aware so destructive commands (rm, mv, chmod, chown) can be scoped to specific directories via a per-pattern `paths` field.

**Architecture:** A new `internal/cmdpath` package provides command descriptors that extract filesystem target paths from command arguments, and path resolution/checking logic. The `patterns.Pattern` struct gains a `Paths` field. Config parsing validates `paths` usage. The hook approval flow gains a new step after safe-pattern matching that checks extracted paths against allowed prefixes.

**Tech Stack:** Go stdlib (`path/filepath`, `strings`, `os`, `regexp`), existing `mvdan.cc/sh/v3` for shell parsing (already a dependency).

**Spec:** `docs/superpowers/specs/2026-03-24-context-aware-paths-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/cmdpath/cmdpath.go` | Create | CommandDescriptor type, descriptor registry, `LookupDescriptor()` |
| `internal/cmdpath/extract.go` | Create | `ExtractTargets` functions for rm, mv, chmod, chown |
| `internal/cmdpath/resolve.go` | Create | Path resolution: tilde expansion, variable expansion, relative path resolution, prefix checking |
| `internal/cmdpath/cmdpath_test.go` | Create | Tests for descriptor registry and lookup |
| `internal/cmdpath/extract_test.go` | Create | Tests for each command's target extraction |
| `internal/cmdpath/resolve_test.go` | Create | Tests for path resolution and prefix checking |
| `internal/patterns/patterns.go` | Modify | Add `Paths []string` field to `Pattern` struct |
| `internal/audit/audit.go` | Modify | Add `PathCheck` struct, `PATH_VIOLATION` code, `Paths` field on `Segment` |
| `internal/config/config.go` | Modify | Parse `paths` from TOML, validate against descriptor registry, detect conflicting path constraints |
| `internal/config/config_test.go` | Modify | Tests for paths config parsing and validation errors |
| `internal/hook/hook.go` | Modify | Add `Paths` to `SafeResult`, add path checking step in `ProcessWithResult()` |
| `internal/hook/hook_test.go` | Modify | Integration tests for path-aware approval |

---

## Task 1: Add `Paths` Field to Pattern Struct

**Files:**
- Modify: `internal/patterns/patterns.go:11-16`
- Test: `internal/patterns/patterns_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/patterns/patterns_test.go`, add:

```go
func TestPatternPathsField(t *testing.T) {
	p := Pattern{
		Regex:   regexp.MustCompile(`^rm\b`),
		Name:    "rm",
		Type:    "simple",
		Pattern: `^rm\b`,
		Paths:   []string{"$PROJECT", "/tmp"},
	}
	if len(p.Paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(p.Paths))
	}
	if p.Paths[0] != "$PROJECT" {
		t.Errorf("expected $PROJECT, got %s", p.Paths[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just test`
Expected: FAIL — `p.Paths` does not exist.

- [ ] **Step 3: Write minimal implementation**

In `internal/patterns/patterns.go`, add `Paths` field to the `Pattern` struct:

```go
type Pattern struct {
	Regex   *regexp.Regexp
	Name    string
	Type    string   // simple, subcommand, command, regex
	Pattern string   // original pattern string
	Paths   []string // allowed path prefixes (empty means no path checking)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `just test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/patterns/patterns.go internal/patterns/patterns_test.go
git commit -m "feat: add Paths field to Pattern struct"
```

---

## Task 2: Add `PathCheck` and `PATH_VIOLATION` to Audit Package

**Files:**
- Modify: `internal/audit/audit.go:16-22,44-51`
- Test: `internal/audit/audit_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/audit/audit_test.go`, add:

```go
func TestPathCheckSerialization(t *testing.T) {
	segment := Segment{
		Command:  "rm /etc/passwd",
		Approved: false,
		Paths: &PathCheck{
			Targets:  []string{"/etc/passwd"},
			Allowed:  []string{"/home/user/project", "/tmp"},
			Approved: false,
		},
		Rejection: &Rejection{
			Code: CodePathViolation,
		},
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("failed to marshal segment: %v", err)
	}

	var decoded Segment
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal segment: %v", err)
	}

	if decoded.Paths == nil {
		t.Fatal("expected Paths to be non-nil")
	}
	if len(decoded.Paths.Targets) != 1 || decoded.Paths.Targets[0] != "/etc/passwd" {
		t.Errorf("unexpected targets: %v", decoded.Paths.Targets)
	}
	if decoded.Paths.Approved {
		t.Error("expected Paths.Approved to be false")
	}
	if decoded.Rejection.Code != "PATH_VIOLATION" {
		t.Errorf("expected PATH_VIOLATION, got %s", decoded.Rejection.Code)
	}
}

func TestPathCheckWithUnresolved(t *testing.T) {
	segment := Segment{
		Command:  "rm $FOO/bar",
		Approved: false,
		Paths: &PathCheck{
			Targets:    []string{},
			Allowed:    []string{"/home/user/project"},
			Unresolved: []string{"$FOO/bar"},
			Approved:   false,
		},
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("failed to marshal segment: %v", err)
	}

	// Verify unresolved is present
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	paths := raw["paths"].(map[string]any)
	if paths["unresolved"] == nil {
		t.Error("expected unresolved to be present")
	}
}

func TestPathCheckOmittedWhenNil(t *testing.T) {
	segment := Segment{
		Command:  "ls",
		Approved: true,
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("failed to marshal segment: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, exists := raw["paths"]; exists {
		t.Error("expected paths to be omitted when nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just test`
Expected: FAIL — `PathCheck` type and `CodePathViolation` constant do not exist.

- [ ] **Step 3: Write minimal implementation**

In `internal/audit/audit.go`, add the constant and types:

```go
// Rejection codes
const (
	CodeCommandSubstitution = "COMMAND_SUBSTITUTION"
	CodeUnparseable         = "UNPARSEABLE"
	CodeDenyMatch           = "DENY_MATCH"
	CodeNoMatch             = "NO_MATCH"
	CodeRewrite             = "REWRITE"
	CodePathViolation       = "PATH_VIOLATION"
)
```

Add `PathCheck` struct and update `Segment`:

```go
// PathCheck contains path resolution and validation details.
type PathCheck struct {
	Targets    []string `json:"targets"`
	Allowed    []string `json:"allowed"`
	Unresolved []string `json:"unresolved,omitempty"`
	Approved   bool     `json:"approved"`
}

// Segment represents a single command segment within a chained command.
type Segment struct {
	Command   string     `json:"command"`
	Approved  bool       `json:"approved"`
	Wrappers  []string   `json:"wrappers,omitempty"`
	Match     *Match     `json:"match,omitempty"`
	Rejection *Rejection `json:"rejection,omitempty"`
	Paths     *PathCheck `json:"paths,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `just test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/audit/audit.go internal/audit/audit_test.go
git commit -m "feat: add PathCheck struct and PATH_VIOLATION rejection code"
```

---

## Task 3: Create Command Descriptor Registry

**Files:**
- Create: `internal/cmdpath/cmdpath.go`
- Create: `internal/cmdpath/cmdpath_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmdpath/cmdpath_test.go`:

```go
package cmdpath

import (
	"testing"
)

func TestLookupDescriptor(t *testing.T) {
	tests := []struct {
		name    string
		command string
		found   bool
	}{
		{"rm is registered", "rm", true},
		{"mv is registered", "mv", true},
		{"chmod is registered", "chmod", true},
		{"chown is registered", "chown", true},
		{"ls is not registered", "ls", false},
		{"git is not registered", "git", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, ok := LookupDescriptor(tt.command)
			if ok != tt.found {
				t.Errorf("LookupDescriptor(%q) found=%v, want %v", tt.command, ok, tt.found)
			}
			if ok && desc.Name != tt.command {
				t.Errorf("LookupDescriptor(%q) name=%q, want %q", tt.command, desc.Name, tt.command)
			}
		})
	}
}

func TestRegisteredCommands(t *testing.T) {
	cmds := RegisteredCommands()
	if len(cmds) != 4 {
		t.Errorf("expected 4 registered commands, got %d", len(cmds))
	}

	expected := map[string]bool{"rm": true, "mv": true, "chmod": true, "chown": true}
	for _, cmd := range cmds {
		if !expected[cmd] {
			t.Errorf("unexpected registered command: %s", cmd)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmdpath/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cmdpath/cmdpath.go`:

```go
// Package cmdpath provides command-specific argument parsing for extracting
// filesystem target paths from shell commands, and path resolution/validation
// against allowed directory prefixes.
package cmdpath

import "sort"

// CommandDescriptor defines how to extract filesystem target paths from a command's arguments.
type CommandDescriptor struct {
	Name           string
	ExtractTargets func(args []string) (targets []string, unresolved []string)
}

// registry maps command names to their descriptors.
var registry = map[string]CommandDescriptor{}

// register adds a descriptor to the registry. Called from extract.go init.
func register(desc CommandDescriptor) {
	registry[desc.Name] = desc
}

// LookupDescriptor returns the descriptor for a command, if registered.
func LookupDescriptor(command string) (CommandDescriptor, bool) {
	desc, ok := registry[command]
	return desc, ok
}

// RegisteredCommands returns a sorted list of all registered command names.
func RegisteredCommands() []string {
	cmds := make([]string, 0, len(registry))
	for name := range registry {
		cmds = append(cmds, name)
	}
	sort.Strings(cmds)
	return cmds
}
```

Create a stub `internal/cmdpath/extract.go` so the registry is populated (we'll fill in extraction logic in Task 4):

```go
package cmdpath

func init() {
	register(CommandDescriptor{
		Name:           "rm",
		ExtractTargets: extractRmTargets,
	})
	register(CommandDescriptor{
		Name:           "mv",
		ExtractTargets: extractMvTargets,
	})
	register(CommandDescriptor{
		Name:           "chmod",
		ExtractTargets: extractChmodTargets,
	})
	register(CommandDescriptor{
		Name:           "chown",
		ExtractTargets: extractChownTargets,
	})
}

// Stub implementations — filled in Task 4
func extractRmTargets(args []string) ([]string, []string)    { return nil, nil }
func extractMvTargets(args []string) ([]string, []string)    { return nil, nil }
func extractChmodTargets(args []string) ([]string, []string) { return nil, nil }
func extractChownTargets(args []string) ([]string, []string) { return nil, nil }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cmdpath/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cmdpath/
git commit -m "feat: add command descriptor registry for path-aware commands"
```

---

## Task 4: Implement Target Extraction for rm and mv

**Files:**
- Modify: `internal/cmdpath/extract.go`
- Create: `internal/cmdpath/extract_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/cmdpath/extract_test.go`:

```go
package cmdpath

import (
	"reflect"
	"testing"
)

func TestExtractRmTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "simple file",
			args:        []string{"foo.txt"},
			wantTargets: []string{"foo.txt"},
		},
		{
			name:        "multiple files",
			args:        []string{"a.txt", "b.txt"},
			wantTargets: []string{"a.txt", "b.txt"},
		},
		{
			name:        "flags are skipped",
			args:        []string{"-rf", "dir/"},
			wantTargets: []string{"dir/"},
		},
		{
			name:        "combined flags",
			args:        []string{"-r", "-f", "dir/"},
			wantTargets: []string{"-r", "-f", "dir/"},
		},
		{
			name:        "double dash",
			args:        []string{"--", "-weird-file"},
			wantTargets: []string{"-weird-file"},
		},
		{
			name:        "double dash with flags before",
			args:        []string{"-rf", "--", "-file1", "-file2"},
			wantTargets: []string{"-file1", "-file2"},
		},
		{
			name:           "shell variable",
			args:           []string{"$HOME/foo"},
			wantUnresolved: []string{"$HOME/foo"},
		},
		{
			name:           "mixed resolved and unresolved",
			args:           []string{"foo.txt", "$DIR/bar"},
			wantTargets:    []string{"foo.txt"},
			wantUnresolved: []string{"$DIR/bar"},
		},
		{
			name:        "absolute path",
			args:        []string{"-f", "/tmp/foo.txt"},
			wantTargets: []string{"/tmp/foo.txt"},
		},
		{
			name:        "glob pattern",
			args:        []string{"*.log"},
			wantTargets: []string{"*.log"},
		},
		{
			name: "no args",
			args: []string{},
		},
		{
			name:        "long flags are skipped",
			args:        []string{"--force", "--recursive", "dir/"},
			wantTargets: []string{"dir/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractRmTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}

func TestExtractMvTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "source and dest",
			args:        []string{"src.txt", "dst.txt"},
			wantTargets: []string{"src.txt", "dst.txt"},
		},
		{
			name:        "flags skipped",
			args:        []string{"-f", "src.txt", "dst.txt"},
			wantTargets: []string{"src.txt", "dst.txt"},
		},
		{
			name:        "double dash",
			args:        []string{"--", "-src", "-dst"},
			wantTargets: []string{"-src", "-dst"},
		},
		{
			name:           "variable in target",
			args:           []string{"src.txt", "$DEST"},
			wantTargets:    []string{"src.txt"},
			wantUnresolved: []string{"$DEST"},
		},
		{
			name:        "long flags",
			args:        []string{"--force", "--backup=numbered", "src", "dst"},
			wantTargets: []string{"src", "dst"},
		},
		{
			name:        "target-directory flag with arg",
			args:        []string{"-t", "/tmp", "src.txt"},
			wantTargets: []string{"/tmp", "src.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractMvTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmdpath/ -run "TestExtractRm|TestExtractMv" -v`
Expected: FAIL — stub functions return nil.

- [ ] **Step 3: Write implementation**

Replace the stubs in `internal/cmdpath/extract.go`:

```go
package cmdpath

import "strings"

func init() {
	register(CommandDescriptor{
		Name:           "rm",
		ExtractTargets: extractRmTargets,
	})
	register(CommandDescriptor{
		Name:           "mv",
		ExtractTargets: extractMvTargets,
	})
	register(CommandDescriptor{
		Name:           "chmod",
		ExtractTargets: extractChmodTargets,
	})
	register(CommandDescriptor{
		Name:           "chown",
		ExtractTargets: extractChownTargets,
	})
}

// isUnresolvable returns true if the arg contains shell variable references
// or other syntax that prevents static path resolution.
func isUnresolvable(arg string) bool {
	return strings.ContainsAny(arg, "$`")
}

// isFlag returns true if the arg looks like a command flag.
func isFlag(arg string) bool {
	return strings.HasPrefix(arg, "-")
}

// isFlagWithArg returns true if the flag expects a following argument.
// This handles flags like -t <dir> for mv, --suffix=<arg>, etc.
// Flags with = (like --backup=numbered) are self-contained and don't consume the next arg.
func isFlagWithArg(command, flag string) bool {
	switch command {
	case "mv":
		return flag == "-t" || flag == "--target-directory" ||
			flag == "-S" || flag == "--suffix"
	case "rm":
		return false // rm has no flags that take arguments
	}
	return false
}

// extractSimpleTargets extracts non-flag args as targets, handling -- and variables.
// Used by commands where all non-flag positional args are filesystem targets (rm, mv).
func extractSimpleTargets(command string, args []string) (targets []string, unresolved []string) {
	pastDoubleDash := false
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !pastDoubleDash && arg == "--" {
			pastDoubleDash = true
			continue
		}

		if !pastDoubleDash && isFlag(arg) {
			// Check if this flag consumes the next argument
			if !strings.Contains(arg, "=") && isFlagWithArg(command, arg) && i+1 < len(args) {
				// The next arg is the flag's value — treat it as a target too
				// since flags like -t <dir> reference filesystem paths
				i++
				nextArg := args[i]
				if isUnresolvable(nextArg) {
					unresolved = append(unresolved, nextArg)
				} else {
					targets = append(targets, nextArg)
				}
			}
			continue
		}

		if isUnresolvable(arg) {
			unresolved = append(unresolved, arg)
		} else {
			targets = append(targets, arg)
		}
	}
	return targets, unresolved
}

func extractRmTargets(args []string) ([]string, []string) {
	return extractSimpleTargets("rm", args)
}

func extractMvTargets(args []string) ([]string, []string) {
	return extractSimpleTargets("mv", args)
}

// Stub implementations for chmod/chown — filled in Task 5
func extractChmodTargets(args []string) ([]string, []string) { return nil, nil }
func extractChownTargets(args []string) ([]string, []string) { return nil, nil }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmdpath/ -run "TestExtractRm|TestExtractMv" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cmdpath/extract.go internal/cmdpath/extract_test.go
git commit -m "feat: implement target extraction for rm and mv"
```

---

## Task 5: Implement Target Extraction for chmod and chown

**Files:**
- Modify: `internal/cmdpath/extract.go`
- Modify: `internal/cmdpath/extract_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/cmdpath/extract_test.go`:

```go
func TestExtractChmodTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "numeric mode",
			args:        []string{"755", "file.sh"},
			wantTargets: []string{"file.sh"},
		},
		{
			name:        "symbolic mode",
			args:        []string{"u+x", "file.sh"},
			wantTargets: []string{"file.sh"},
		},
		{
			name:        "4-digit numeric mode",
			args:        []string{"0644", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "complex symbolic mode",
			args:        []string{"go-rwx", "secret.key"},
			wantTargets: []string{"secret.key"},
		},
		{
			name:        "multiple files",
			args:        []string{"644", "a.txt", "b.txt"},
			wantTargets: []string{"a.txt", "b.txt"},
		},
		{
			name:        "flags before mode",
			args:        []string{"-R", "755", "dir/"},
			wantTargets: []string{"dir/"},
		},
		{
			name:        "double dash",
			args:        []string{"755", "--", "-weird-file"},
			wantTargets: []string{"-weird-file"},
		},
		{
			name:           "variable in path",
			args:           []string{"755", "$DIR/file"},
			wantUnresolved: []string{"$DIR/file"},
		},
		{
			name: "no args",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractChmodTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}

func TestExtractChownTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "user only",
			args:        []string{"root", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "user:group",
			args:        []string{"root:wheel", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "user with dot",
			args:        []string{"user.name", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "multiple files",
			args:        []string{"nobody", "a.txt", "b.txt"},
			wantTargets: []string{"a.txt", "b.txt"},
		},
		{
			name:        "flags before owner",
			args:        []string{"-R", "root", "dir/"},
			wantTargets: []string{"dir/"},
		},
		{
			name:        "double dash",
			args:        []string{"root", "--", "-weird-file"},
			wantTargets: []string{"-weird-file"},
		},
		{
			name:           "variable in path",
			args:           []string{"root", "$DIR/file"},
			wantUnresolved: []string{"$DIR/file"},
		},
		{
			name:        "colon-only group",
			args:        []string{":wheel", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name: "no args",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractChownTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmdpath/ -run "TestExtractChmod|TestExtractChown" -v`
Expected: FAIL — stub functions return nil.

- [ ] **Step 3: Write implementation**

In `internal/cmdpath/extract.go`, replace the chmod/chown stubs and add the mode/owner parsing helpers:

```go
import (
	"regexp"
	"strings"
)

// chmodModePattern matches numeric modes (e.g., 755, 0644) and symbolic modes (e.g., u+x, go-rwx).
var chmodModePattern = regexp.MustCompile(`^[0-7]{3,4}$|^[ugoa]*[+-=][rwxXst]+$`)

// chownOwnerPattern matches user, user:group, :group, and user: patterns.
var chownOwnerPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]*(:[a-zA-Z0-9._-]*)?$`)

func extractChmodTargets(args []string) ([]string, []string) {
	return extractWithFirstArgSkip("chmod", args, chmodModePattern)
}

func extractChownTargets(args []string) ([]string, []string) {
	return extractWithFirstArgSkip("chown", args, chownOwnerPattern)
}

// extractWithFirstArgSkip extracts targets where the first non-flag positional arg
// is a special value (mode for chmod, owner for chown) that should be skipped.
func extractWithFirstArgSkip(command string, args []string, skipPattern *regexp.Regexp) (targets []string, unresolved []string) {
	pastDoubleDash := false
	skippedFirst := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !pastDoubleDash && arg == "--" {
			pastDoubleDash = true
			continue
		}

		if !pastDoubleDash && isFlag(arg) {
			continue
		}

		// Skip the first positional arg if it matches the pattern (mode/owner)
		if !pastDoubleDash && !skippedFirst && skipPattern.MatchString(arg) {
			skippedFirst = true
			continue
		}
		skippedFirst = true

		if isUnresolvable(arg) {
			unresolved = append(unresolved, arg)
		} else {
			targets = append(targets, arg)
		}
	}
	return targets, unresolved
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmdpath/ -run "TestExtractChmod|TestExtractChown" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cmdpath/extract.go internal/cmdpath/extract_test.go
git commit -m "feat: implement target extraction for chmod and chown"
```

---

## Task 6: Implement Path Resolution and Prefix Checking

**Files:**
- Create: `internal/cmdpath/resolve.go`
- Create: `internal/cmdpath/resolve_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/cmdpath/resolve_test.go`:

```go
package cmdpath

import (
	"testing"
)

func TestExpandPathVariables(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		cwd     string
		gitRoot string
		want    []string
	}{
		{
			name:    "expand $PROJECT",
			paths:   []string{"$PROJECT"},
			cwd:     "/home/user/project",
			gitRoot: "/home/user/project",
			want:    []string{"/home/user/project"},
		},
		{
			name:    "expand $PROJECT_ROOT",
			paths:   []string{"$PROJECT_ROOT"},
			cwd:     "/home/user/project/.claude/worktrees/feat",
			gitRoot: "/home/user/project",
			want:    []string{"/home/user/project"},
		},
		{
			name:    "literal path unchanged",
			paths:   []string{"/tmp"},
			cwd:     "/home/user/project",
			gitRoot: "/home/user/project",
			want:    []string{"/tmp"},
		},
		{
			name:    "mixed variables and literals",
			paths:   []string{"$PROJECT", "/tmp", "$PROJECT_ROOT"},
			cwd:     "/home/user/project/.claude/worktrees/feat",
			gitRoot: "/home/user/project",
			want:    []string{"/home/user/project/.claude/worktrees/feat", "/tmp", "/home/user/project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPathVariables(tt.paths, tt.cwd, tt.gitRoot)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d paths, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("path[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantPath   string
		wantResolved bool
	}{
		{
			name:         "tilde only",
			path:         "~",
			wantPath:     "", // will be $HOME
			wantResolved: true,
		},
		{
			name:         "tilde with path",
			path:         "~/foo/bar",
			wantPath:     "", // will be $HOME/foo/bar
			wantResolved: true,
		},
		{
			name:         "tilde with user",
			path:         "~bob/foo",
			wantPath:     "~bob/foo",
			wantResolved: false,
		},
		{
			name:         "no tilde",
			path:         "/absolute/path",
			wantPath:     "/absolute/path",
			wantResolved: true,
		},
		{
			name:         "relative path",
			path:         "relative/path",
			wantPath:     "relative/path",
			wantResolved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, resolved := ExpandTilde(tt.path)
			if resolved != tt.wantResolved {
				t.Errorf("resolved = %v, want %v", resolved, tt.wantResolved)
			}
			if !resolved {
				return
			}
			// For tilde paths, just verify it starts with a non-tilde character
			if tt.path == "~" || (len(tt.path) > 1 && tt.path[1] == '/') {
				if len(got) == 0 || got[0] == '~' {
					t.Errorf("tilde was not expanded: %q", got)
				}
			}
		})
	}
}

func TestResolveTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []string
		cwd     string
		want    []string
	}{
		{
			name:    "absolute path unchanged",
			targets: []string{"/tmp/foo"},
			cwd:     "/home/user",
			want:    []string{"/tmp/foo"},
		},
		{
			name:    "relative path resolved",
			targets: []string{"foo.txt"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/foo.txt"},
		},
		{
			name:    "dot-dot resolved",
			targets: []string{"../bar.txt"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/bar.txt"},
		},
		{
			name:    "dot resolved",
			targets: []string{"./foo.txt"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/foo.txt"},
		},
		{
			name:    "glob base directory",
			targets: []string{"*.log"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/*.log"},
		},
		{
			name:    "glob with directory",
			targets: []string{"subdir/*.log"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/subdir/*.log"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveTargets(tt.targets, tt.cwd)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d targets, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("target[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCheckPathPrefixes(t *testing.T) {
	tests := []struct {
		name    string
		targets []string
		allowed []string
		ok      bool
	}{
		{
			name:    "target under allowed",
			targets: []string{"/home/user/project/foo.txt"},
			allowed: []string{"/home/user/project"},
			ok:      true,
		},
		{
			name:    "target is exactly allowed",
			targets: []string{"/home/user/project"},
			allowed: []string{"/home/user/project"},
			ok:      true,
		},
		{
			name:    "target outside allowed",
			targets: []string{"/etc/passwd"},
			allowed: []string{"/home/user/project"},
			ok:      false,
		},
		{
			name:    "one of multiple allowed",
			targets: []string{"/tmp/foo"},
			allowed: []string{"/home/user/project", "/tmp"},
			ok:      true,
		},
		{
			name:    "mixed: one in, one out",
			targets: []string{"/home/user/project/ok.txt", "/etc/bad"},
			allowed: []string{"/home/user/project"},
			ok:      false,
		},
		{
			name:    "no targets is ok",
			targets: []string{},
			allowed: []string{"/home/user/project"},
			ok:      true,
		},
		{
			name:    "prefix must be directory boundary",
			targets: []string{"/home/user/project-other/foo.txt"},
			allowed: []string{"/home/user/project"},
			ok:      false,
		},
		{
			name:    "trailing slash on allowed",
			targets: []string{"/home/user/project/foo.txt"},
			allowed: []string{"/home/user/project/"},
			ok:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckPathPrefixes(tt.targets, tt.allowed)
			if got != tt.ok {
				t.Errorf("CheckPathPrefixes(%v, %v) = %v, want %v", tt.targets, tt.allowed, got, tt.ok)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmdpath/ -run "TestExpand|TestResolve|TestCheck" -v`
Expected: FAIL — functions do not exist.

- [ ] **Step 3: Write implementation**

Create `internal/cmdpath/resolve.go`:

```go
package cmdpath

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPathVariables expands $PROJECT and $PROJECT_ROOT in path expressions.
func ExpandPathVariables(paths []string, cwd, gitRoot string) []string {
	result := make([]string, len(paths))
	for i, p := range paths {
		switch p {
		case "$PROJECT":
			result[i] = cwd
		case "$PROJECT_ROOT":
			result[i] = gitRoot
		default:
			result[i] = p
		}
	}
	return result
}

// ExpandTilde expands ~ to $HOME. Returns the expanded path and whether it was resolvable.
// ~user paths are not resolvable and return (original, false).
func ExpandTilde(path string) (string, bool) {
	if !strings.HasPrefix(path, "~") {
		return path, true
	}
	// ~ or ~/...
	if path == "~" || strings.HasPrefix(path, "~/") {
		home := os.Getenv("HOME")
		if home == "" {
			return path, false
		}
		if path == "~" {
			return home, true
		}
		return filepath.Join(home, path[2:]), true
	}
	// ~user/... — cannot resolve
	return path, false
}

// ResolveTargets resolves relative paths against cwd and cleans all paths.
func ResolveTargets(targets []string, cwd string) []string {
	result := make([]string, len(targets))
	for i, t := range targets {
		if filepath.IsAbs(t) {
			result[i] = filepath.Clean(t)
		} else {
			result[i] = filepath.Clean(filepath.Join(cwd, t))
		}
	}
	return result
}

// CheckPathPrefixes checks that every target is under at least one allowed prefix.
// Uses directory-boundary-aware prefix matching (not just string prefix).
func CheckPathPrefixes(targets []string, allowed []string) bool {
	for _, target := range targets {
		if !isUnderAnyPrefix(target, allowed) {
			return false
		}
	}
	return true
}

// isUnderAnyPrefix checks if target is under any of the allowed prefixes.
func isUnderAnyPrefix(target string, allowed []string) bool {
	cleanTarget := filepath.Clean(target)
	for _, prefix := range allowed {
		cleanPrefix := filepath.Clean(prefix)
		if cleanTarget == cleanPrefix {
			return true
		}
		// Ensure directory boundary: prefix must end with separator
		if !strings.HasSuffix(cleanPrefix, string(filepath.Separator)) {
			cleanPrefix += string(filepath.Separator)
		}
		if strings.HasPrefix(cleanTarget, cleanPrefix) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmdpath/ -run "TestExpand|TestResolve|TestCheck" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cmdpath/resolve.go internal/cmdpath/resolve_test.go
git commit -m "feat: implement path resolution and prefix checking"
```

---

## Task 7: Parse `paths` from TOML Config and Validate

**Files:**
- Modify: `internal/config/config.go:80-171`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestPathsFieldParsedOnSimple(t *testing.T) {
	configData := []byte(`
[[commands.simple]]
name = "destructive"
commands = ["rm", "mv"]
paths = ["$PROJECT", "/tmp"]
`)
	cfg, err := LoadConfig(configData)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// rm and mv should each have paths
	for _, p := range cfg.SafeCommands {
		if p.Name != "destructive" {
			continue
		}
		if len(p.Paths) != 2 {
			t.Errorf("pattern %q: expected 2 paths, got %d", p.Name, len(p.Paths))
		}
		if len(p.Paths) >= 2 && (p.Paths[0] != "$PROJECT" || p.Paths[1] != "/tmp") {
			t.Errorf("pattern %q: unexpected paths %v", p.Name, p.Paths)
		}
	}
}

func TestPathsFieldOmittedIsNil(t *testing.T) {
	configData := []byte(`
[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	cfg, err := LoadConfig(configData)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	for _, p := range cfg.SafeCommands {
		if len(p.Paths) != 0 {
			t.Errorf("pattern %q: expected nil paths, got %v", p.Name, p.Paths)
		}
	}
}

func TestPathsOnRegexFails(t *testing.T) {
	configData := []byte(`
[[commands.regex]]
pattern = "^rm\\b"
name = "rm-regex"
paths = ["$PROJECT"]
`)
	_, err := LoadConfig(configData)
	if err == nil {
		t.Fatal("expected error for paths on regex pattern")
	}
	if !strings.Contains(err.Error(), "paths") {
		t.Errorf("error should mention paths: %v", err)
	}
}

func TestPathsOnSubcommandFails(t *testing.T) {
	configData := []byte(`
[[commands.subcommand]]
command = "git"
subcommands = ["status"]
paths = ["$PROJECT"]
`)
	_, err := LoadConfig(configData)
	if err == nil {
		t.Fatal("expected error for paths on subcommand pattern")
	}
	if !strings.Contains(err.Error(), "paths") {
		t.Errorf("error should mention paths: %v", err)
	}
}

func TestPathsOnUnknownCommandFails(t *testing.T) {
	configData := []byte(`
[[commands.simple]]
name = "unknown"
commands = ["curl"]
paths = ["$PROJECT"]
`)
	_, err := LoadConfig(configData)
	if err == nil {
		t.Fatal("expected error for paths on unknown command")
	}
	if !strings.Contains(err.Error(), "no path descriptor") {
		t.Errorf("error should mention no path descriptor: %v", err)
	}
}

func TestPathsConflictingPatternsFails(t *testing.T) {
	configData := []byte(`
[[commands.simple]]
name = "constrained"
commands = ["rm"]
paths = ["$PROJECT"]

[[commands.simple]]
name = "unconstrained"
commands = ["rm", "ls"]
`)
	_, err := LoadConfig(configData)
	if err == nil {
		t.Fatal("expected error for conflicting path constraints on rm")
	}
	if !strings.Contains(err.Error(), "conflicting") {
		t.Errorf("error should mention conflicting: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run "TestPaths" -v`
Expected: FAIL — paths not parsed, validation not implemented.

- [ ] **Step 3: Write implementation**

In `internal/config/config.go`, modify the `simple` case in `parseSection()` to extract `paths`:

```go
case "simple":
	entries := toMapSlice(value)
	for i, entry := range entries {
		name, _ := entry["name"].(string)
		cmds := toStringSlice(entry["commands"])
		paths := toStringSlice(entry["paths"])
		if len(cmds) == 0 {
			if name != "" {
				return nil, fmt.Errorf("%s.simple[%d] %q: \"commands\" field is required and must not be empty", sectionName, i, name)
			}
			return nil, fmt.Errorf("%s.simple[%d]: \"commands\" field is required and must not be empty", sectionName, i)
		}
		for _, cmd := range cmds {
			var pattern string
			var patternName string
			if isWrapper {
				pattern = patterns.BuildWrapperPattern(cmd, nil)
				patternName = cmd
			} else {
				pattern = patterns.BuildSimplePattern(cmd)
				patternName = name
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
			}
			result = append(result, patterns.Pattern{Regex: re, Name: patternName, Type: "simple", Pattern: pattern, Paths: paths})
		}
	}
```

In the `regex` case, add validation that `paths` is not present:

```go
case "regex":
	entries := toMapSlice(value)
	for i, entry := range entries {
		pattern, _ := entry["pattern"].(string)
		patternName, _ := entry["name"].(string)
		paths := toStringSlice(entry["paths"])
		if len(paths) > 0 {
			if patternName != "" {
				return nil, fmt.Errorf("%s.regex[%d] %q: \"paths\" is not supported on regex patterns", sectionName, i, patternName)
			}
			return nil, fmt.Errorf("%s.regex[%d]: \"paths\" is not supported on regex patterns", sectionName, i)
		}
		// ... rest unchanged
```

In the `subcommand` case, add validation that `paths` is not present:

```go
case "subcommand":
	entries := toMapSlice(value)
	for i, entry := range entries {
		cmd, _ := entry["command"].(string)
		paths := toStringSlice(entry["paths"])
		if len(paths) > 0 {
			return nil, fmt.Errorf("%s.subcommand[%d] %q: \"paths\" is not supported on subcommand patterns", sectionName, i, cmd)
		}
		// ... rest unchanged
```

Add a new `validatePathConstraints` function and call it from `loadConfigWithIncludes` after parsing all sections. Add the import for `cmdpath`:

```go
import (
	"github.com/dgerlanc/mmi/internal/cmdpath"
)
```

```go
// validatePathConstraints checks that:
// 1. Commands with paths have registered descriptors
// 2. No command appears in both path-constrained and unconstrained patterns
func validatePathConstraints(safeCommands []patterns.Pattern) error {
	// Track which commands have paths and which don't
	type pathState struct {
		hasPaths    bool
		patternName string
	}
	commandPaths := make(map[string][]pathState)

	for _, p := range safeCommands {
		if p.Type != "simple" {
			continue
		}

		// Extract the command name from the pattern.
		// Simple patterns are "^cmd\b", so extract between ^ and \b.
		cmd := extractCommandFromSimplePattern(p.Pattern)
		if cmd == "" {
			continue
		}

		// Validate descriptor exists if paths are set
		if len(p.Paths) > 0 {
			if _, ok := cmdpath.LookupDescriptor(cmd); !ok {
				return fmt.Errorf("command %q has paths but no path descriptor registered (supported: %v)",
					cmd, cmdpath.RegisteredCommands())
			}
		}

		commandPaths[cmd] = append(commandPaths[cmd], pathState{
			hasPaths:    len(p.Paths) > 0,
			patternName: p.Name,
		})
	}

	// Check for conflicts
	for cmd, states := range commandPaths {
		if len(states) < 2 {
			continue
		}
		hasConstrained := false
		hasUnconstrained := false
		for _, s := range states {
			if s.hasPaths {
				hasConstrained = true
			} else {
				hasUnconstrained = true
			}
		}
		if hasConstrained && hasUnconstrained {
			return fmt.Errorf("conflicting path constraints for command %q: appears in both path-constrained and unconstrained patterns", cmd)
		}
	}

	return nil
}

// extractCommandFromSimplePattern extracts the command name from a simple pattern like "^cmd\b".
func extractCommandFromSimplePattern(pattern string) string {
	// Simple patterns are "^<escaped_cmd>\b"
	if !strings.HasPrefix(pattern, "^") || !strings.HasSuffix(pattern, `\b`) {
		return ""
	}
	escaped := pattern[1 : len(pattern)-2]
	// Unescape regex metacharacters (QuoteMeta escapes . + etc)
	return regexp.QuoteMeta(escaped) // This is wrong — we need to unescape
}
```

Actually, a simpler approach: track the raw command name during parsing rather than reverse-engineering it from the pattern. Modify `parseSection` to return the command names alongside patterns, or store the raw command name in `Pattern`. The cleanest approach: add a `Command` field to `Pattern` that stores the raw command name for simple patterns. But to avoid scope creep, instead pass command names directly during validation. The simplest approach is to do the validation inline during parsing:

Replace the approach above. Instead, validate inside the `simple` case of `parseSection`:

```go
// After building the pattern, validate paths
if len(paths) > 0 && !isWrapper {
	if _, ok := cmdpath.LookupDescriptor(cmd); !ok {
		return nil, fmt.Errorf("%s.simple[%d] %q: command %q has paths but no path descriptor registered (supported: %v)",
			sectionName, i, name, cmd, cmdpath.RegisteredCommands())
	}
}
```

For conflict detection, add a post-parse validation in `loadConfigWithIncludes`. Track command→hasPaths during simple pattern parsing by adding a field to `Pattern`:

Actually, the simplest correct approach: add a `Command string` field to `Pattern` (only populated for simple patterns) so we can look it up later. But that adds a field just for validation. Instead, validate conflicts by checking every pair of simple patterns in the final config. We can extract the command from simple patterns reliably since we know the format. Let me use a different approach — track it in config parsing:

In `loadConfigWithIncludes`, after all sections are parsed, call:

```go
if err := validatePathConflicts(cfg.SafeCommands); err != nil {
	return nil, err
}
```

With:

```go
func validatePathConflicts(safeCommands []patterns.Pattern) error {
	// For simple patterns, the pattern is "^<quotemeta(cmd)>\b"
	// We can reverse QuoteMeta for simple command names (no metacharacters)
	type entry struct {
		hasPaths bool
		name     string
	}
	cmdEntries := make(map[string][]entry)

	for _, p := range safeCommands {
		if p.Type != "simple" {
			continue
		}
		// Extract raw command: strip "^" prefix and "\b" suffix
		raw := p.Pattern
		if strings.HasPrefix(raw, "^") && strings.HasSuffix(raw, `\b`) {
			cmd := raw[1 : len(raw)-2]
			// Simple commands don't have regex metacharacters, so QuoteMeta is identity
			cmdEntries[cmd] = append(cmdEntries[cmd], entry{
				hasPaths: len(p.Paths) > 0,
				name:     p.Name,
			})
		}
	}

	for cmd, entries := range cmdEntries {
		if len(entries) < 2 {
			continue
		}
		hasConstrained := false
		hasUnconstrained := false
		for _, e := range entries {
			if e.hasPaths {
				hasConstrained = true
			} else {
				hasUnconstrained = true
			}
		}
		if hasConstrained && hasUnconstrained {
			return fmt.Errorf("conflicting path constraints for command %q: appears in both path-constrained and unconstrained patterns", cmd)
		}
	}
	return nil
}
```

Call this at the end of `loadConfigWithIncludes`, before `return cfg, nil`:

```go
// Validate path constraint conflicts
if err := validatePathConflicts(cfg.SafeCommands); err != nil {
	return nil, fmt.Errorf("path validation failed: %w", err)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run "TestPaths" -v`
Expected: PASS

- [ ] **Step 5: Run all tests to verify nothing is broken**

Run: `just test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: parse and validate paths field in config"
```

---

## Task 8: Add `Paths` to `SafeResult` and Path Checking in Hook

**Files:**
- Modify: `internal/hook/hook.go:351-372` (SafeResult, CheckSafe)
- Modify: `internal/hook/hook.go:229-348` (ProcessWithResult segment loop)

- [ ] **Step 1: Write the failing tests**

Add to `internal/hook/hook_test.go`:

```go
func TestCheckSafeReturnsPaths(t *testing.T) {
	p := patterns.Pattern{
		Regex:   regexp.MustCompile(`^rm\b`),
		Name:    "rm",
		Type:    "simple",
		Pattern: `^rm\b`,
		Paths:   []string{"$PROJECT", "/tmp"},
	}

	result := CheckSafe("rm foo.txt", []patterns.Pattern{p})
	if !result.Matched {
		t.Fatal("expected match")
	}
	if len(result.Paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(result.Paths))
	}
}

func TestCheckSafeNoPathsWhenNil(t *testing.T) {
	p := patterns.Pattern{
		Regex:   regexp.MustCompile(`^ls\b`),
		Name:    "ls",
		Type:    "simple",
		Pattern: `^ls\b`,
	}

	result := CheckSafe("ls -la", []patterns.Pattern{p})
	if !result.Matched {
		t.Fatal("expected match")
	}
	if len(result.Paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(result.Paths))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/hook/ -run "TestCheckSafe(Returns|NoPaths)" -v`
Expected: FAIL — `SafeResult` has no `Paths` field.

- [ ] **Step 3: Add Paths to SafeResult and CheckSafe**

In `internal/hook/hook.go`, update `SafeResult`:

```go
type SafeResult struct {
	Matched bool
	Name    string
	Type    string   // simple, subcommand, regex, command
	Pattern string
	Paths   []string // allowed path prefixes from config (nil = no path checking)
}
```

Update `CheckSafe` to pass through `Paths`:

```go
func CheckSafe(cmd string, safeCommands []patterns.Pattern) SafeResult {
	for _, p := range safeCommands {
		if p.Regex.MatchString(cmd) {
			return SafeResult{
				Matched: true,
				Name:    p.Name,
				Type:    p.Type,
				Pattern: p.Pattern,
				Paths:   p.Paths,
			}
		}
	}
	return SafeResult{Matched: false}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hook/ -run "TestCheckSafe(Returns|NoPaths)" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hook/hook.go internal/hook/hook_test.go
git commit -m "feat: thread Paths through SafeResult in CheckSafe"
```

---

## Task 9: Integrate Path Checking into ProcessWithResult

**Files:**
- Modify: `internal/hook/hook.go:229-348` (segment evaluation loop in ProcessWithResult)
- Modify: `internal/hook/hook_test.go`

- [ ] **Step 1: Write the failing integration tests**

Add to `internal/hook/hook_test.go`:

```go
func TestProcessWithResultPathApproved(t *testing.T) {
	cleanup := setupPathTestConfig(t, `
[[commands.simple]]
name = "rm-safe"
commands = ["rm"]
paths = ["$PROJECT", "/tmp"]
`)
	defer cleanup()

	logPath, logCleanup := setupTestAudit(t)
	defer logCleanup()

	input := makeHookInput("rm /tmp/foo.txt", "/home/user/project")
	result := ProcessWithResult(strings.NewReader(input))

	if !result.Approved {
		t.Errorf("expected approved, got rejected: %s", result.Output)
	}

	// Verify audit log contains path check
	entries := readAuditEntries(t, logPath)
	if len(entries) == 0 {
		t.Fatal("expected audit entry")
	}
	if entries[0].Segments[0].Paths == nil {
		t.Error("expected Paths in audit segment")
	}
	if !entries[0].Segments[0].Paths.Approved {
		t.Error("expected Paths.Approved = true")
	}
}

func TestProcessWithResultPathViolation(t *testing.T) {
	cleanup := setupPathTestConfig(t, `
[[commands.simple]]
name = "rm-safe"
commands = ["rm"]
paths = ["$PROJECT"]
`)
	defer cleanup()

	logPath, logCleanup := setupTestAudit(t)
	defer logCleanup()

	input := makeHookInput("rm /etc/passwd", "/home/user/project")
	result := ProcessWithResult(strings.NewReader(input))

	if result.Approved {
		t.Error("expected rejected for path violation")
	}

	// Should be "ask" not "deny"
	var output hook.Output
	json.Unmarshal([]byte(result.Output), &output)
	if output.HookSpecificOutput.PermissionDecision != "ask" {
		t.Errorf("expected ask decision, got %s", output.HookSpecificOutput.PermissionDecision)
	}

	entries := readAuditEntries(t, logPath)
	if len(entries) == 0 {
		t.Fatal("expected audit entry")
	}
	seg := entries[0].Segments[0]
	if seg.Rejection == nil || seg.Rejection.Code != audit.CodePathViolation {
		t.Errorf("expected PATH_VIOLATION rejection, got %+v", seg.Rejection)
	}
}

func TestProcessWithResultPathUnresolved(t *testing.T) {
	cleanup := setupPathTestConfig(t, `
[[commands.simple]]
name = "rm-safe"
commands = ["rm"]
paths = ["$PROJECT"]
`)
	defer cleanup()

	_, logCleanup := setupTestAudit(t)
	defer logCleanup()

	input := makeHookInput("rm $SOME_VAR/file", "/home/user/project")
	result := ProcessWithResult(strings.NewReader(input))

	if result.Approved {
		t.Error("expected rejected for unresolvable path")
	}
}

func TestProcessWithResultNoPathsNoCheck(t *testing.T) {
	cleanup := setupPathTestConfig(t, `
[[commands.simple]]
name = "safe"
commands = ["ls"]
`)
	defer cleanup()

	_, logCleanup := setupTestAudit(t)
	defer logCleanup()

	// ls with any path should be approved (no paths constraint)
	input := makeHookInput("ls /etc/passwd", "/home/user/project")
	result := ProcessWithResult(strings.NewReader(input))

	if !result.Approved {
		t.Errorf("expected approved (no path checking), got rejected")
	}
}

func TestProcessWithResultProjectVariable(t *testing.T) {
	cleanup := setupPathTestConfig(t, `
[[commands.simple]]
name = "rm-safe"
commands = ["rm"]
paths = ["$PROJECT"]
`)
	defer cleanup()

	_, logCleanup := setupTestAudit(t)
	defer logCleanup()

	// rm a file inside the project (relative path)
	input := makeHookInput("rm foo.txt", "/home/user/project")
	result := ProcessWithResult(strings.NewReader(input))

	if !result.Approved {
		t.Errorf("expected approved for file in project, got rejected")
	}
}

// Helper functions

func setupPathTestConfig(t *testing.T, configContent string) func() {
	t.Helper()
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}
	config.Reset()
	config.Init()
	return func() {
		os.Unsetenv("MMI_CONFIG")
		config.Reset()
	}
}

func makeHookInput(command, cwd string) string {
	input := Input{
		SessionID:      "test-session",
		TranscriptPath: "/tmp/transcript",
		Cwd:            cwd,
		PermissionMode: "default",
		HookEventName:  "PreToolUse",
		ToolName:       "Bash",
		ToolInput:      ToolInputData{Command: command},
		ToolUseID:      "test-tool-use",
	}
	data, _ := json.Marshal(input)
	return string(data)
}

func readAuditEntries(t *testing.T, logPath string) []audit.Entry {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read audit log: %v", err)
	}
	var entries []audit.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry audit.Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to parse audit entry: %v", err)
		}
		entries = append(entries, entry)
	}
	return entries
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/hook/ -run "TestProcessWithResultPath" -v`
Expected: FAIL — path checking not yet implemented in ProcessWithResult.

- [ ] **Step 3: Write implementation**

In `internal/hook/hook.go`, add import:

```go
import (
	"github.com/dgerlanc/mmi/internal/cmdpath"
)
```

In the segment evaluation loop, after `CheckSafe` succeeds and before adding the approved segment to `auditSegments`, add the path checking step. Replace the block starting at "Approved segment" (after the `!safeResult.Matched` check):

```go
		// Path checking: if pattern has paths, validate target directories
		if len(safeResult.Paths) > 0 {
			pathCheckResult := checkPaths(coreCmd, safeResult.Paths, input.Cwd)
			if !pathCheckResult.Approved {
				logger.Debug("rejected by path check", "command", coreCmd, "targets", pathCheckResult.Targets, "allowed", pathCheckResult.Allowed, "unresolved", pathCheckResult.Unresolved)
				overallApproved = false
				auditSegments = append(auditSegments, audit.Segment{
					Command:  segment,
					Approved: false,
					Wrappers: wrappers,
					Match: &audit.Match{
						Type:    safeResult.Type,
						Name:    safeResult.Name,
						Pattern: safeResult.Pattern,
					},
					Paths: &pathCheckResult,
					Rejection: &audit.Rejection{
						Code: audit.CodePathViolation,
					},
				})
				continue
			}

			// Approved with path check
			logger.Debug("matched pattern with path check", "command", coreCmd, "pattern", safeResult.Name)
			auditSegments = append(auditSegments, audit.Segment{
				Command:  segment,
				Approved: true,
				Wrappers: wrappers,
				Match: &audit.Match{
					Type:    safeResult.Type,
					Name:    safeResult.Name,
					Pattern: safeResult.Pattern,
				},
				Paths: &pathCheckResult,
			})
		} else {
			// Approved without path check (existing behavior)
			logger.Debug("matched pattern", "command", coreCmd, "pattern", safeResult.Name)
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
		}

		if len(wrappers) > 0 {
			reasons = append(reasons, strings.Join(wrappers, "+")+" + "+safeResult.Name)
		} else {
			reasons = append(reasons, safeResult.Name)
		}
```

Add the `checkPaths` function:

```go
// checkPaths validates that a command's filesystem targets are within allowed paths.
func checkPaths(coreCmd string, configPaths []string, cwd string) audit.PathCheck {
	// Split the core command to get command name and args
	parts := splitCommandArgs(coreCmd)
	if len(parts) == 0 {
		return audit.PathCheck{Approved: true}
	}

	commandName := parts[0]
	args := parts[1:]

	// Look up the command descriptor
	desc, ok := cmdpath.LookupDescriptor(commandName)
	if !ok {
		// No descriptor — shouldn't happen if config validation passed, but fail closed
		return audit.PathCheck{
			Approved: false,
		}
	}

	// Extract targets from args
	targets, unresolved := desc.ExtractTargets(args)

	// Expand tilde in targets
	var expandedTargets []string
	for _, t := range targets {
		expanded, ok := cmdpath.ExpandTilde(t)
		if !ok {
			unresolved = append(unresolved, t)
			continue
		}
		expandedTargets = append(expandedTargets, expanded)
	}

	// If any args are unresolvable, fail closed
	if len(unresolved) > 0 {
		allowed := cmdpath.ExpandPathVariables(configPaths, cwd, resolveGitRoot(cwd))
		return audit.PathCheck{
			Targets:    cmdpath.ResolveTargets(expandedTargets, cwd),
			Allowed:    allowed,
			Unresolved: unresolved,
			Approved:   false,
		}
	}

	// Resolve relative paths
	resolvedTargets := cmdpath.ResolveTargets(expandedTargets, cwd)

	// Expand config path variables
	allowed := cmdpath.ExpandPathVariables(configPaths, cwd, resolveGitRoot(cwd))

	// Check prefix
	approved := len(resolvedTargets) == 0 || cmdpath.CheckPathPrefixes(resolvedTargets, allowed)

	return audit.PathCheck{
		Targets:  resolvedTargets,
		Allowed:  allowed,
		Approved: approved,
	}
}

// splitCommandArgs splits a command string into command name and arguments.
// Uses the shell parser for correctness with quoted args.
func splitCommandArgs(cmd string) []string {
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil || len(prog.Stmts) == 0 {
		// Fallback to simple split
		return strings.Fields(cmd)
	}

	stmt := prog.Stmts[0]
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return strings.Fields(cmd)
	}

	printer := syntax.NewPrinter()
	var parts []string
	for _, word := range call.Args {
		var buf strings.Builder
		printer.Print(&buf, word)
		parts = append(parts, buf.String())
	}
	return parts
}

// resolveGitRoot returns the git repository root for the given directory.
// Returns cwd if git root cannot be determined.
func resolveGitRoot(cwd string) string {
	// Walk up looking for .git directory or file (worktree uses a .git file)
	dir := cwd
	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return dir
			}
			// .git file (worktree) — read it to find the real git dir
			data, err := os.ReadFile(gitPath)
			if err == nil {
				content := strings.TrimSpace(string(data))
				if strings.HasPrefix(content, "gitdir: ") {
					gitDir := strings.TrimPrefix(content, "gitdir: ")
					if !filepath.IsAbs(gitDir) {
						gitDir = filepath.Join(dir, gitDir)
					}
					// The common dir is typically two levels up from .git/worktrees/<name>
					commonDir := filepath.Clean(filepath.Join(gitDir, "..", ".."))
					if info, err := os.Stat(commonDir); err == nil && info.IsDir() {
						return commonDir
					}
				}
			}
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd // Reached filesystem root
		}
		dir = parent
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hook/ -run "TestProcessWithResultPath" -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `just test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/hook/hook.go internal/hook/hook_test.go
git commit -m "feat: integrate path checking into command approval flow"
```

---

## Task 10: End-to-End Integration Tests

**Files:**
- Modify: `main_test.go`

- [ ] **Step 1: Write end-to-end tests**

Add to `main_test.go`:

```go
func TestE2EPathAwareApproval(t *testing.T) {
	pathConfig := `
[[commands.simple]]
name = "destructive"
commands = ["rm", "mv"]
paths = ["$PROJECT", "/tmp"]

[[commands.simple]]
name = "safe"
commands = ["ls", "cat"]
`
	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)
	defer os.Unsetenv("MMI_CONFIG")
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(pathConfig), 0644); err != nil {
		t.Fatal(err)
	}
	config.Reset()
	defer config.Reset()
	config.Init()

	tests := []struct {
		name     string
		command  string
		cwd      string
		approved bool
	}{
		{
			name:     "rm inside project approved",
			command:  "rm foo.txt",
			cwd:      "/home/user/project",
			approved: true,
		},
		{
			name:     "rm in /tmp approved",
			command:  "rm /tmp/scratch.txt",
			cwd:      "/home/user/project",
			approved: true,
		},
		{
			name:     "rm outside project rejected",
			command:  "rm /etc/passwd",
			cwd:      "/home/user/project",
			approved: false,
		},
		{
			name:     "rm with relative escape rejected",
			command:  "rm ../../etc/passwd",
			cwd:      "/home/user/project",
			approved: false,
		},
		{
			name:     "mv inside project approved",
			command:  "mv old.txt new.txt",
			cwd:      "/home/user/project",
			approved: true,
		},
		{
			name:     "mv to outside rejected",
			command:  "mv foo.txt /opt/bar.txt",
			cwd:      "/home/user/project",
			approved: false,
		},
		{
			name:     "ls anywhere approved (no paths)",
			command:  "ls /etc",
			cwd:      "/home/user/project",
			approved: true,
		},
		{
			name:     "rm with variable rejected",
			command:  "rm $SOME_DIR/file",
			cwd:      "/home/user/project",
			approved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := hook.Input{
				SessionID:      "test",
				TranscriptPath: "/tmp/t",
				Cwd:            tt.cwd,
				PermissionMode: "default",
				HookEventName:  "PreToolUse",
				ToolName:       "Bash",
				ToolInput:      hook.ToolInputData{Command: tt.command},
				ToolUseID:      "test",
			}
			data, _ := json.Marshal(input)
			result := hook.ProcessWithResult(bytes.NewReader(data))
			if result.Approved != tt.approved {
				t.Errorf("command %q: approved=%v, want %v (output: %s)",
					tt.command, result.Approved, tt.approved, result.Output)
			}
		})
	}
}
```

- [ ] **Step 2: Run the e2e tests**

Run: `go test -run TestE2EPathAware -v`
Expected: PASS

- [ ] **Step 3: Run the full test suite**

Run: `just test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add main_test.go
git commit -m "test: add end-to-end tests for path-aware approval"
```

---

## Task 11: Update SPEC.md

**Files:**
- Modify: `docs/SPEC.md`

- [ ] **Step 1: Update Section 3.2 (Configuration)**

Add `Paths` to the Config struct documentation:

```go
type Config struct {
    WrapperPatterns  []patterns.Pattern  // Layer 2: Safe prefixes
    SafeCommands     []patterns.Pattern  // Layer 3: Allowlisted commands
    DenyPatterns     []patterns.Pattern  // Layer 1: Always rejected
    SubshellAllowAll bool                // Allow command substitution
    RewriteRules     []patterns.RewriteRule // Command rewrite suggestions
}
```

Add `Paths` to the Pattern struct:

```go
type Pattern struct {
    Regex   *regexp.Regexp
    Name    string
    Type    string
    Pattern string
    Paths   []string  // Allowed path prefixes (nil = no path checking)
}
```

- [ ] **Step 2: Update Section 4.1 (Processing Flow)**

Add step 6 to the flow diagram:

```
6. If matched pattern has paths:
   a. Extract target paths (command descriptor)
   b. Resolve relative paths against cwd
   c. Check against allowed prefixes
```

- [ ] **Step 3: Update Section 5.2 (Configuration Format)**

Add `paths` field documentation and path variables (`$PROJECT`, `$PROJECT_ROOT`).

- [ ] **Step 4: Update Section 8.7 (Rejection Codes)**

Add `PATH_VIOLATION` to the rejection codes table:

| Code | Description | When Used |
|------|-------------|-----------|
| `PATH_VIOLATION` | Target path outside allowed directories | Command safe-listed but targets path not in `paths` prefixes |

- [ ] **Step 5: Add new section for Command Descriptor Registry**

Document the registry, supported commands (rm, mv, chmod, chown), and the `internal/cmdpath` package.

- [ ] **Step 6: Commit**

```bash
git add docs/SPEC.md
git commit -m "docs: update SPEC.md with path checking feature"
```
