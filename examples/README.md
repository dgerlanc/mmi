# MMI Configuration Examples

This directory contains example configuration files for different use cases.

## Using Examples

Copy any example file to your MMI config directory:

```bash
cp examples/python.toml ~/.config/mmi/config.toml
```

Or use them as includes in your main config:

```toml
# ~/.config/mmi/config.toml
include = ["python.toml"]

# Add your custom patterns here
[[commands.simple]]
name = "custom"
commands = ["my-tool"]
```

## Available Examples

### minimal.toml
A minimal configuration with only essential commands. Good starting point
for security-conscious users who want to explicitly allow each command.

### python.toml
Python development focused configuration:
- pytest, python, ruff, uv, pip
- Virtual environment activation
- Common Python tooling

### node.toml
Node.js/JavaScript development focused configuration:
- npm, npx, yarn, pnpm
- Common frontend tooling

### rust.toml
Rust development focused configuration:
- cargo (build, test, run, etc.)
- maturin for Python bindings
- Common Rust tooling

### strict.toml
A strict profile that only allows read-only commands.
Useful for CI environments or when maximum caution is needed.

## Profiles

You can also use these as profiles by copying them to the profiles directory:

```bash
mkdir -p ~/.config/mmi/profiles
cp examples/strict.toml ~/.config/mmi/profiles/strict.toml
```

Then use with:
```bash
mmi --profile strict
# or
export MMI_PROFILE=strict
```

## Creating Custom Configs

Use `mmi init` for an interactive configuration generator, or copy
one of these examples and modify it to fit your needs.
