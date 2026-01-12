# Default recipe - show available commands
default:
    @just --list

# Build the binary
build:
    go build -o mmi

# Run tests with verbose output
test:
    go test -v

# Run tests with coverage summary
coverage:
    go test -cover

# Run tests with coverage report file
coverage-report:
    go test -v -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

# Run tests with verbose output and coverage
test-coverage:
    go test -v -cover

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
