# MMI Improvement Suggestions

## Code Quality & Structure

1. ~~**Extract pattern building into a separate package**~~ ✓ IMPLEMENTED - Pattern building functions are now in the `patterns` package.

2. ~~**Add structured logging**~~ ✓ IMPLEMENTED - Structured logging using `log/slog` is now available.

3. ~~**Consider using a command parser library**~~ ✓ IMPLEMENTED - Replaced manual regex-based command chain splitting with `mvdan.cc/sh/v3` shell parser for more robust handling of quoted strings, redirections, and shell syntax.

## Security Enhancements

4. ~~**Add audit logging**~~ ✓ IMPLEMENTED - Approval/rejection decisions are logged to `~/.local/share/mmi/audit.log`.

5. **Support for command argument validation** - Currently, simple commands allow any arguments once the command name is approved. This feature would add argument-level pattern matching to prevent dangerous flag combinations. For example:
   - Block `rm -rf /` or `rm -rf ~` while still allowing `rm file.txt`
   - Block `chmod 777` while allowing `chmod 644`
   - Block `curl | sh` patterns that pipe untrusted scripts to shell
   - Require certain flags (e.g., only allow `grep` with `--include` to limit scope)

   Implementation could use a new config section like:
   ```toml
   [[allow.validated]]
   name = "safe-rm"
   command = "rm"
   allowed_args = ["-r", "-f", "-i", "-v"]
   blocked_patterns = ["-rf /", "-rf ~", "-rf $HOME"]
   ```

6. ~~**Add a deny list**~~ ✓ IMPLEMENTED - Deny list support is available for explicit denials that override approvals.

## Feature Additions

7. ~~**Dry-run mode**~~ ✓ IMPLEMENTED - Use `--dry-run` flag to test command approval without JSON output.

8. ~~**Verbose mode**~~ ✓ IMPLEMENTED - Use `--verbose` or `-v` flag to enable debug logging.

9. ~~**Config validation command**~~ ✓ IMPLEMENTED - Use `mmi validate` to check config syntax and show compiled patterns.

10. ~~**Interactive config generator**~~ ✓ IMPLEMENTED - Use `mmi init` to create a starter config interactively.

## Testing Improvements

11. ~~**Add fuzzing tests**~~ ✓ IMPLEMENTED - Fuzzing tests are available using Go's built-in fuzzing.

12. ~~**Add benchmarks**~~ ✓ IMPLEMENTED - Benchmarks are available for regex compilation and matching.

13. ~~**Generate coverage report as HTML**~~ ✓ IMPLEMENTED - HTML coverage report available via justfile recipe.

## Documentation

14. ~~**Add examples directory**~~ ✓ IMPLEMENTED - Example configurations for different use cases are available in the `examples/` directory.

15. ~~**Document regex pattern syntax**~~ ✓ IMPLEMENTED - Pattern syntax documentation is available in `docs/PATTERNS.md`.

16. ~~**Add CHANGELOG.md**~~ ✓ IMPLEMENTED - Version history is tracked in `CHANGELOG.md`.

## Configuration

17. ~~**Support config includes**~~ ✓ IMPLEMENTED - Config can be split into multiple files using includes.

18. ~~**Add config profiles**~~ ✓ IMPLEMENTED - Multiple named profiles are supported and switchable via environment variable.

19. **Config schema validation** - Create a JSON Schema (or TOML equivalent) that describes the config file structure. This enables:
   - **IDE autocompletion**: Editors like VS Code can suggest valid keys and values as you type
   - **Inline documentation**: Hover over config keys to see descriptions and valid options
   - **Error highlighting**: Catch typos and invalid config before running `mmi validate`
   - **Type checking**: Ensure arrays contain the right types, required fields are present, etc.

   Implementation would involve:
   1. Creating a JSON Schema file (e.g., `schema/mmi-config.schema.json`)
   2. Adding a `$schema` directive to example configs
   3. Optionally publishing the schema to [SchemaStore](https://www.schemastore.org/) for automatic IDE detection

## Build & Distribution

20. ~~**Add GitHub Actions CI**~~ ✓ IMPLEMENTED - GitHub Actions workflow automates testing, linting, and releases.

21. ~~**Create releases with goreleaser**~~ ✓ IMPLEMENTED - GoReleaser configuration is available in `.goreleaser.yaml`.

22. **Add Homebrew formula** - Make installation easier on macOS.

23. ~~**Add shell completions**~~ ✓ IMPLEMENTED - Shell completions are available via `mmi completion` command.

## Additional Improvements

### Security Enhancements

24. **Audit log rotation and retention** - Currently audit logs can grow unbounded. This feature would add:
   - Size-based or date-based log rotation (e.g., rotate at 10MB or daily)
   - Configurable retention policy (keep last N log files)
   - Prevention of disk exhaustion attacks
   - Could use `lumberjack` library or similar

25. **Secure audit log permissions** - Audit logs are currently created with `0644` (world-readable). This feature would:
   - Change file creation to `0600` (user-only read/write)
   - Change directory creation from `0755` to `0700`
   - Prevent information leakage to other users on shared systems

26. **Regex complexity validation** - User-supplied regex patterns are not validated for ReDoS vulnerability. This feature would:
   - Validate patterns for catastrophic backtracking potential
   - Add optional timeout for regex operations
   - Warn on potentially problematic patterns during `mmi validate`
   - Example problematic pattern: `(a+)+b` could hang on inputs like `aaaaaaaaaaaaaaaaaaaaaaaaaaaa!`

### Testing Improvements

27. **Improve hook package test coverage** - The hook package currently has only 21.1% test coverage (compared to 100% for the patterns package). This would add:
   - Direct unit tests for `ProcessWithResult()` function
   - Tests for audit logging integration
   - Tests for all decision paths (approved, denied, unparseable, non-Bash)
   - Tests for error recovery scenarios

28. **Add concurrent access tests** - Test thread safety under load:
   - Concurrent audit log writes
   - Concurrent config reads
   - Race condition detection using `go test -race`
   - Stress testing with parallel command evaluations

29. **Add Unicode and edge case tests** - Expand test coverage for edge cases:
   - Non-ASCII characters in commands (emoji, international characters)
   - Very long command strings (performance boundaries)
   - Pathological inputs beyond current fuzz corpus
   - Memory usage validation under stress

### Feature Additions

30. **Audit log analysis command** - New `mmi audit` subcommand to analyze audit history:
   - Show approval/rejection statistics (e.g., "85% approved, 15% rejected")
   - Filter by date range, command pattern, or decision
   - Identify frequently rejected commands (potential config improvements)
   - Export reports in JSON or CSV format

31. **Audit log integrity verification** - Tamper detection for audit trail:
   - Option to sign or hash audit entries
   - Verification command to detect modifications
   - Chain hashing (each entry includes hash of previous)
   - Useful for compliance and forensics

32. **Remote audit log support** - Send audit entries to external services:
   - Syslog protocol support for enterprise logging systems
   - Webhook support for custom integrations
   - Configurable backends (multiple destinations)
   - Async delivery with local buffering on failure

### Documentation

33. **Security policy (SECURITY.md)** - Document security considerations:
   - Threat model and security boundaries
   - Responsible disclosure process
   - Known limitations and non-goals
   - Security best practices for configuration

34. **Contributing guidelines (CONTRIBUTING.md)** - Development documentation:
   - Code style requirements and Go idioms
   - Testing requirements (coverage thresholds, test types)
   - Pull request process and review criteria
   - Development environment setup instructions

35. **Test coverage badges in README** - Visibility into code quality:
   - Add coverage badge showing overall percentage
   - Link to detailed HTML coverage report
   - Encourage maintaining high coverage standards
   - Could use codecov.io or similar service
