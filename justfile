# Default recipe - show available commands
default:
    @just --list

# Build the binary
build:
    go build -o mmi

# Run tests with verbose output
test:
    go test -v

# Run tests with coverage
coverage:
    go test -cover

# Run tests with verbose output and coverage
test-coverage:
    go test -v -cover

# Format Go code
fmt:
    go fmt ./...

# Run Go vet linter
vet:
    go vet ./...

# Clean build artifacts
clean:
    go clean
    rm -f mmi

# Run all checks (fmt, vet, test)
check: fmt vet test

# Build and install (default: /usr/local/bin)
install prefix="/usr/local": build
    mv mmi {{prefix}}/bin/

# Tidy go.mod
tidy:
    go mod tidy
