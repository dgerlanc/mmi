# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.2] - 2026-02-06

## [0.2.1] - 2026-02-06

## [0.2.0] - 2026-02-06

## [0.1.4] - 2026-02-05

## [0.1.3] - 2026-01-21

## [0.1.2] - 2026-01-18

### Added
- **v1 audit log format**: Rich metadata including version, tool_use_id, session_id, duration_ms, per-segment match details, and working directory
- **Pre-commit support**: `.pre-commit-config.yaml` for automated checks

### Changed
- Command substitution (`$(...)` and backticks) now checked per segment instead of rejecting entire command
- Hook input JSON schema updated to match Claude Code format (session_id, tool_use_id)
- Audit log timestamps use 1 decimal place for milliseconds
- Deny list checks consolidated to single location

### Fixed
- Piped commands now log all segments in audit entries

### Removed
- **Multiple profiles**: Removed `--profile` flag and `MMI_PROFILE` env var (use `MMI_CONFIG` to point to different config directories instead)

## [0.1.1] - 2026-01-13

### Changed
- Pin goreleaser to v2.7.0

### Fixed
- **Security**: Reject unparseable bash commands instead of falling back to single segment treatment

### Removed
- Homebrew formula generation from GoReleaser config

## [0.1.0] - 2026-01-13

### Added

#### CLI & Commands
- **Cobra CLI framework**: Proper CLI with subcommands, flags, and help text
- **`mmi validate`**: Validate configuration and display compiled patterns
- **`mmi init`**: Interactive configuration generator
- **`mmi completion`**: Shell completion scripts for bash, zsh, fish, and PowerShell
- **`--dry-run` flag**: Test command approval without JSON output
- **`--verbose` / `-v` flag**: Enable debug logging
- **`--no-audit-log` flag**: Disable audit logging

#### Configuration
- **Deny list**: Patterns that are always rejected (checked before approval)
  - `[[deny.simple]]` for command names
  - `[[deny.regex]]` for custom patterns
- **Config includes**: Split config across multiple files with `include = [...]`

#### Logging & Debugging
- **Structured logging**: Debug output using Go's `log/slog`
- **Audit logging**: JSON-lines log of all decisions to `~/.local/share/mmi/audit.log`
  - Timestamps, commands, approval status, and reasons
  - Disable with `--no-audit-log`

#### Testing & Quality
- **Fuzzing tests**: Fuzz tests for critical functions
- **Benchmarks**: Performance benchmarks for pattern matching
- **HTML coverage report**: `just coverage-html` generates visual coverage report

#### Build & Distribution
- **GoReleaser configuration**: Cross-platform release builds
  - Linux, macOS, Windows on amd64 and arm64
  - Automatic GitHub releases on tag push
  - Homebrew formula generation
- **GitHub Actions release workflow**: Automated releases on version tags

#### Documentation & Examples
- **Example configurations**: Ready-to-use configs for different use cases
  - `minimal.toml` - Bare-bones for security-conscious users
  - `python.toml` - Python development
  - `node.toml` - Node.js development
  - `rust.toml` - Rust development
  - `strict.toml` - Read-only commands only
- **Pattern syntax documentation**: Reference for writing custom patterns

#### justfile Recipes
- `just fuzz` - Run fuzz tests
- `just bench` - Run benchmarks
- `just coverage-html` - Generate HTML coverage report
- `just release-test` - Test GoReleaser configuration

#### Core Features
- Basic command approval hook for Claude Code
- Pattern-based command matching
- Support for wrappers (env, timeout, etc.)
- TOML configuration format

### Changed
- Refactored monolithic `main.go` into modular package structure:
  - `cmd/` - CLI commands
  - `internal/hook/` - Core approval logic
  - `internal/config/` - Configuration loading
  - `internal/logger/` - Structured logging
  - `internal/audit/` - Audit logging
- Improved command parsing with `mvdan.cc/sh/v3` shell parser

### Fixed
- Better handling of complex shell constructs (loops, conditionals, etc.)
