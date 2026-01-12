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
