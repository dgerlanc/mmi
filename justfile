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
