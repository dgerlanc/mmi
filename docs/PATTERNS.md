# MMI Pattern Syntax Reference

This document explains the pattern syntax used in MMI configuration files.

## Pattern Types

MMI supports several ways to define command patterns:

### 1. Simple Commands (`[[*.simple]]`)

Match commands by name with any arguments.

```toml
[[commands.simple]]
name = "python"
commands = ["python", "pytest", "ruff"]
```

This generates patterns like `^python\b`, `^pytest\b`, etc.

The `\b` word boundary ensures "python" doesn't match "python3" (you'd need to add that explicitly).

### 2. Subcommands (`[[*.subcommand]]`)

Match commands that require specific subcommands.

```toml
[[commands.subcommand]]
command = "git"
subcommands = ["status", "log", "diff", "add"]
flags = ["-C <arg>"]
```

This generates a pattern like:
```
^git\s+(-C\s*\S+\s+)?(status|log|diff|add)\b
```

The `flags` field allows optional flags before the subcommand.

### 3. Wrappers (`[[wrappers.command]]`)

Match wrapper commands that prefix other commands.

```toml
[[wrappers.command]]
command = "timeout"
flags = ["<arg>"]
```

Generates: `^timeout\s+(\S+\s+)?`

The `<arg>` placeholder matches any non-whitespace argument.

### 4. Raw Regex (`[[*.regex]]`)

For complex patterns that can't be expressed with the other types.

```toml
[[commands.regex]]
pattern = '^(true|false|exit(\s+\d+)?)$'
name = "shell builtin"
```

## Go Regex Syntax

MMI uses Go's `regexp` package, which uses RE2 syntax. Key differences from other regex flavors:

### Supported Features

| Pattern | Meaning |
|---------|---------|
| `.` | Any character except newline |
| `*` | Zero or more of preceding |
| `+` | One or more of preceding |
| `?` | Zero or one of preceding |
| `^` | Start of string |
| `$` | End of string |
| `\s` | Whitespace |
| `\S` | Non-whitespace |
| `\w` | Word character (alphanumeric + underscore) |
| `\d` | Digit |
| `\b` | Word boundary |
| `[abc]` | Character class |
| `[^abc]` | Negated character class |
| `(a|b)` | Alternation |
| `(?:...)` | Non-capturing group |

### NOT Supported (RE2 limitations)

- Backreferences (`\1`, `\2`)
- Lookahead/lookbehind (`(?=...)`, `(?!...)`, `(?<=...)`, `(?<!...)`)
- Possessive quantifiers (`*+`, `++`)
- Atomic groups (`(?>...)`)

## Common Patterns

### Match command at start
```toml
pattern = '^git\b'
```

### Match command with specific flag
```toml
pattern = '^rm\s+(-[rRfF]+\s+)*/'  # rm -rf /
```

### Match environment variable assignment
```toml
pattern = '^[A-Z_][A-Z0-9_]*=\S*$'
```

### Match virtual environment path
```toml
pattern = '^(\.\./)*\.?venv/bin/'
```

## Testing Patterns

### Using mmi validate

```bash
mmi validate
```

Shows all compiled patterns so you can verify the regex.

### Using dry-run mode

```bash
echo '{"tool_name":"Bash","tool_input":{"command":"git status"}}' | mmi --dry-run
```

### Using Go regexp online

Test patterns at: https://regex101.com/ (select "Golang" flavor)

### In Go code

```go
import "regexp"

re := regexp.MustCompile(`^git\s+(status|log)\b`)
fmt.Println(re.MatchString("git status"))  // true
fmt.Println(re.MatchString("git push"))    // false
```

## Pattern Evaluation Order

1. **Shell parsing** - command must be valid, parseable shell syntax
2. **For each segment:**
   - **Dangerous patterns** (command substitution like `$()` or backticks) are checked
   - **Deny patterns** are checked (against segment and core command)
   - **Wrappers** are stripped from the segment
   - **Safe commands** are matched against the core command
3. **All segments are evaluated** regardless of whether earlier segments are rejected (for complete audit logging)

A command is approved only if:
- It IS valid, parseable shell syntax (incomplete loops, unclosed quotes are rejected)
- It does NOT contain command substitution in any segment
- It does NOT match any deny pattern
- The core command (after stripping wrappers) matches a safe pattern

**Note:** Shell loops (`while`, `for`, `if`, etc.) must be complete. MMI extracts and validates their inner commands individually.

## Escaping Special Characters

In TOML, use single quotes for regex to avoid double-escaping:

```toml
# Good - single quotes
pattern = '^rm\s+-rf\s+/'

# Also works - double quotes with escaping
pattern = "^rm\\s+-rf\\s+/"
```

## Tips

1. **Start patterns with `^`** to anchor to the beginning of the command
2. **Use `\b` for word boundaries** to avoid partial matches
3. **Use `\s+` not just ` `** to handle tabs and multiple spaces
4. **Test with verbose mode** (`mmi -v`) to see pattern matching in action
5. **Keep patterns simple** - complex patterns are harder to audit
