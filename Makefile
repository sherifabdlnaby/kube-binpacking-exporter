.PHONY: help test test-verbose test-coverage coverage-html coverage-func vet lint build clean

# Default target shows available commands
help:
	@echo "Available targets:"
	@echo "  make test              - Run all tests"
	@echo "  make test-verbose      - Run tests with verbose output"
	@echo "  make test-coverage     - Run tests and generate coverage report"
	@echo "  make coverage-html     - Generate HTML coverage report and open in browser"
	@echo "  make coverage-func     - Show per-function coverage summary"
	@echo "  make vet               - Run go vet"
	@echo "  make lint              - Run golangci-lint (requires installation)"
	@echo "  make build             - Build the binary"
	@echo "  make clean             - Remove build artifacts and coverage files"

# Run all tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	go test -v -race ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@echo ""
	@echo "Coverage summary:"
	go tool cover -func=coverage.out | tail -1

# Generate HTML coverage report
coverage-html: test-coverage
	@echo "Generating HTML coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Opening coverage report in browser..."
	@which open > /dev/null && open coverage.html || \
	which xdg-open > /dev/null && xdg-open coverage.html || \
	echo "Please open coverage.html in your browser"

# Show per-function coverage
coverage-func: test-coverage
	@echo "Per-function coverage:"
	go tool cover -func=coverage.out

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Run golangci-lint (if installed)
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

# Build binary
build:
	@echo "Building binary..."
	go build -o kube-cluster-binpacking-exporter .

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f kube-cluster-binpacking-exporter
	rm -f coverage.out coverage.html
