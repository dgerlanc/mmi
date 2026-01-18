# Default recipe - show available commands
default:
    @just --list

# Build the binary
build:
    go build -o mmi

# Run tests with verbose output (excludes fuzz tests)
test:
    go test -v -skip "^Fuzz"

# Run tests with coverage summary (excludes fuzz tests)
coverage:
    go test -cover -skip "^Fuzz"

# Run tests with coverage report file (excludes fuzz tests)
coverage-report:
    go test -v -skip "^Fuzz" -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

# Generate HTML coverage report (excludes fuzz tests)
coverage-html:
    go test -skip "^Fuzz" -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report written to coverage.html"

# Run tests with verbose output and coverage (excludes fuzz tests)
test-coverage:
    go test -v -cover -skip "^Fuzz"

# Format Go code
fmt:
    go fmt ./...

# Check formatting without modifying files
fmt-check:
    @test -z "$(gofmt -l .)" || (echo "Code is not formatted. Run 'just fmt'" && gofmt -d . && exit 1)

# Run Go vet linter
vet:
    go vet ./...

# Clean build artifacts
clean:
    go clean
    rm -f mmi coverage.out

# Run all checks (fmt-check, vet, test)
check: fmt-check vet test

# Run CI checks locally (matches GitHub Actions)
ci: fmt-check coverage-report build

# Build and install (default: /usr/local/bin)
install prefix="/usr/local": build
    mv mmi {{prefix}}/bin/

# Tidy go.mod
tidy:
    go mod tidy

# Run benchmarks
bench:
    go test -bench=. -benchmem ./...

# Run benchmarks with comparison output (useful for perf regression testing)
bench-compare count="5":
    go test -bench=. -benchmem -count={{count}} ./... | tee bench.txt

# Run fuzz tests (default: 30s per target)
fuzz time="30s":
    go test -fuzz=FuzzSplitCommandChain -fuzztime={{time}} .
    go test -fuzz=FuzzProcess -fuzztime={{time}} .
    go test -fuzz=FuzzStripWrappers -fuzztime={{time}} .
    go test -fuzz=FuzzCheckSafe -fuzztime={{time}} .
    go test -fuzz=FuzzCheckDeny -fuzztime={{time}} .

# Run a specific fuzz test
fuzz-one target time="30s":
    go test -fuzz={{target}} -fuzztime={{time}} .

# Test goreleaser configuration (dry run)
release-test:
    goreleaser check
    goreleaser release --snapshot --clean

# Create a new release (updates changelog, tags, and pushes)
release version:
    @just _release-check
    @just _release-changelog {{version}}
    @just _release-tag {{version}}
    @echo "Release v{{version}} complete! GitHub Actions will build and publish."

# Pre-release checks
_release-check:
    @echo "Running pre-release checks..."
    @test -z "$(git status --porcelain)" || (echo "Error: Working directory not clean" && exit 1)
    @test "$(git branch --show-current)" = "main" || (echo "Error: Must be on main branch" && exit 1)
    @just ci
    @echo "All pre-release checks passed."

# Update changelog for release
_release-changelog version:
    @echo "Updating CHANGELOG.md for v{{version}}..."
    @sed -i '' 's/## \[Unreleased\]/## [Unreleased]\n\n## [{{version}}] - '"$(date +%Y-%m-%d)"'/' CHANGELOG.md
    git add CHANGELOG.md
    git commit -m "Release v{{version}}"

# Create and push release tag
_release-tag version:
    @echo "Creating tag v{{version}}..."
    git tag "v{{version}}"
    git push origin main
    git push origin "v{{version}}"

# Dry-run release (shows what would happen without making changes)
release-dry-run version:
    @echo "=== DRY RUN: Release v{{version}} ==="
    @just _release-check
    @echo "Would update CHANGELOG.md with version {{version}} and date $(date +%Y-%m-%d)"
    @echo "Would commit: 'Release v{{version}}'"
    @echo "Would create tag: v{{version}}"
    @echo "Would push to origin"
    @echo "=== DRY RUN COMPLETE ==="
