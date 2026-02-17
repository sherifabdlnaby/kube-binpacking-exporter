.PHONY: help test test-verbose test-coverage coverage-html coverage-func vet lint lint-install ci build clean

# Build variables (single source of truth for CI, Docker, local builds)
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build flags (centralized ldflags)
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

# golangci-lint version (must match CI)
GOLANGCI_LINT_VERSION = v2.9.0

# Default target shows available commands
help:
	@echo "Available targets:"
	@echo "  make build             - Build the binary with version info"
	@echo "  make test              - Run all tests"
	@echo "  make test-verbose      - Run tests with verbose output"
	@echo "  make test-coverage     - Run tests and generate coverage report"
	@echo "  make coverage-html     - Generate HTML coverage report and open in browser"
	@echo "  make coverage-func     - Show per-function coverage summary"
	@echo "  make vet               - Run go vet"
	@echo "  make lint              - Run golangci-lint"
	@echo "  make lint-install      - Install golangci-lint $(GOLANGCI_LINT_VERSION)"
	@echo "  make ci                - Run all CI checks (build, vet, test, lint)"
	@echo "  make clean             - Remove build artifacts and coverage files"
	@echo ""
	@echo "Build with custom version:"
	@echo "  make build VERSION=v1.2.3 COMMIT=abc123 DATE=2024-01-01T00:00:00Z"

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

# Install golangci-lint at the version used in CI
lint-install:
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	@echo "Installed: $$(golangci-lint --version)"

# Run golangci-lint
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Run 'make lint-install' to install $(GOLANGCI_LINT_VERSION)" && exit 1)
	@INSTALLED_VERSION=$$(golangci-lint --version | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -n1); \
	if [ "$$INSTALLED_VERSION" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "WARNING: golangci-lint version mismatch"; \
		echo "  Installed: $$INSTALLED_VERSION"; \
		echo "  Expected:  $(GOLANGCI_LINT_VERSION) (CI version)"; \
		echo "  Run 'make lint-install' to install the correct version"; \
		echo ""; \
	fi
	golangci-lint run

# Run all CI checks locally
ci: build vet test lint
	@echo ""
	@echo "✓ All CI checks passed!"

# Build binary with version info
build:
	@echo "Building kube-cluster-binpacking-exporter $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o kube-cluster-binpacking-exporter .

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f kube-cluster-binpacking-exporter
	rm -f coverage.out coverage.html
