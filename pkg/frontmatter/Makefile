# Makefile for libs/frontmatter
# Go library build automation

.PHONY: all build test lint clean help

# Default target
all: test lint

## build: Build the package (verify compilation)
build:
	@echo "Building frontmatter package..."
	go build ./...

## test: Run all tests
test:
	@echo "Running tests..."
	go test -v ./... -coverprofile=coverage.out
	@echo "Coverage report:"
	@go tool cover -func=coverage.out | tail -1

## test-race: Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -v ./... -race -coverprofile=coverage.out

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

## clean: Remove build artifacts and cache
clean:
	@echo "Cleaning build artifacts..."
	rm -f coverage.out
	go clean -cache -testcache
	@echo "Clean complete."

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
