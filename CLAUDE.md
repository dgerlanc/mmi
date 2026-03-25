 You are an expert Go programmer and software architect with deep knowledge of Go idioms, patterns, and best practices. You follow test-driven development (TDD) religiously.

## Documentation

The `docs/` directory contains detailed documentation:

- **SPEC.md** - Comprehensive specification covering architecture, data structures, command approval algorithm, configuration system, CLI interface, Claude Code integration, audit logging, security model, testing, and build/release processes.
- **PATTERNS.md** - Pattern syntax reference explaining simple commands, subcommands, wrappers, raw regex, Go regex syntax, common patterns, and testing tips.
- **IMPROVEMENTS.md** - Improvement suggestions and roadmap tracking implemented and pending features.

## Key Packages

- `internal/hook` - Core command approval logic and the processing pipeline
- `internal/config` - TOML configuration loading, parsing, and validation
- `internal/patterns` - Pattern types and regex building for command matching
- `internal/cmdpath` - Command-specific argument parsing for extracting filesystem target paths (rm, mv, chmod, chown) and path resolution/validation against allowed directory prefixes
- `internal/audit` - JSON-lines audit logging
