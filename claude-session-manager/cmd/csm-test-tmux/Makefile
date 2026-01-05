# Makefile for csm-test-tmux

# Version information
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION)-$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)"

# Output binary
BINARY := csm-test-tmux
INSTALL_PATH := $(HOME)/bin

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := $(GOCMD) fmt

.PHONY: all build test lint clean install uninstall help

## all: Build the binary
all: build

## build: Compile the binary with version information
build:
	@echo "Building $(BINARY) $(VERSION)..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY) .
	@echo "✅ Build complete: ./$(BINARY)"

## test: Run all tests (foundation + CLI)
test:
	@echo "Running tests..."
	@cd ../.. && $(GOTEST) -v -cover ./internal/testutil/...
	@echo "✅ All tests passed"

## lint: Run go vet and go fmt
lint:
	@echo "Running linters..."
	@cd ../.. && $(GOVET) ./internal/testutil/... ./cmd/csm-test-tmux/...
	@cd ../.. && $(GOFMT) ./internal/testutil/... ./cmd/csm-test-tmux/...
	@echo "✅ Linters passed"

## install: Build and install to ~/bin (or $INSTALL_PATH)
install: build
	@echo "Installing $(BINARY) to $(INSTALL_PATH)..."
	@mkdir -p $(INSTALL_PATH)
	@cp $(BINARY) $(INSTALL_PATH)/
	@echo "✅ Installed: $(INSTALL_PATH)/$(BINARY)"
	@echo ""
	@echo "Add $(INSTALL_PATH) to PATH if not already:"
	@echo "  export PATH=\"\$$PATH:$(INSTALL_PATH)\""

## uninstall: Remove installed binary
uninstall:
	@echo "Removing $(INSTALL_PATH)/$(BINARY)..."
	@rm -f $(INSTALL_PATH)/$(BINARY)
	@echo "✅ Uninstalled"

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY)
	@echo "✅ Clean complete"

## version: Show version information
version:
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"

## help: Show this help message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

# Default target explanation
.DEFAULT_GOAL := help
