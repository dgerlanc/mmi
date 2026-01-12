# MMI Improvement Suggestions

## Code Quality & Structure

1. **Extract pattern building into a separate package** - The pattern building functions (`buildSimplePattern`, `buildSubcommandPattern`, etc.) could be a separate `patterns` package for better modularity and testability.

2. **Add structured logging** - Replace implicit error handling with structured logging (using `log/slog`) to help users debug why commands are rejected.

3. ~~**Consider using a command parser library**~~ ✓ IMPLEMENTED - Replaced manual regex-based command chain splitting with `mvdan.cc/sh/v3` shell parser for more robust handling of quoted strings, redirections, and shell syntax.

## Security Enhancements

4. **Add audit logging** - Log all approval/rejection decisions to a file for security auditing (`~/.local/share/mmi/audit.log`).

5. **Support for command argument validation** - Currently only command names are validated. Adding argument pattern validation would prevent risky argument combinations (e.g., `rm -rf /`).

6. **Add a deny list** - Support explicit denials that override approvals for dangerous patterns.

## Feature Additions

7. **Dry-run mode** - Add `--dry-run` flag to test what commands would be approved without actually hooking into Claude Code.

8. **Verbose mode** - Add `--verbose` or `-v` flag to explain why a command was approved/rejected (useful for debugging configuration).

9. **Config validation command** - Add `mmi validate` to check config syntax and show compiled patterns.

10. **Interactive config generator** - Add `mmi init` to create a starter config interactively.

## Testing Improvements

11. **Add fuzzing tests** - Use Go's built-in fuzzing to find edge cases in command parsing.

12. **Add benchmarks** - Benchmark the regex compilation and matching for performance analysis.

13. **Generate coverage report as HTML** - Extend justfile to output `go test -coverprofile=coverage.out && go tool cover -html=coverage.out`.

## Documentation

14. **Add examples directory** - Include example configurations for different use cases (Python-only, Node-only, minimal, etc.).

15. **Document regex pattern syntax** - The config format supports regex but there's no documentation on how to write custom patterns.

16. **Add CHANGELOG.md** - Track version history and breaking changes.

## Configuration

17. **Support config includes** - Allow splitting config into multiple files (`include = ["git.toml", "python.toml"]`).

18. **Add config profiles** - Support multiple named profiles switchable via environment variable.

19. **Config schema validation** - Add JSON Schema or similar for IDE autocompletion in config files.

## Build & Distribution

20. **Add GitHub Actions CI** - Automate testing, linting, and releases.

21. **Create releases with goreleaser** - Automatically build binaries for multiple platforms.

22. **Add Homebrew formula** - Make installation easier on macOS.

23. **Add shell completions** - Generate bash/zsh/fish completions for any CLI subcommands.
