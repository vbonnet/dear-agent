.PHONY: build test test-bdd lint install clean

# Version information
VERSION ?= 2.0.0-dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Ldflags for version injection
LDFLAGS_CSM = -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

LDFLAGS_REAPER = -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

LDFLAGS_AGM = -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

build:
	go build -ldflags="$(LDFLAGS_CSM)" -o bin/csm ./cmd/csm
	go build -ldflags="$(LDFLAGS_REAPER)" -o bin/csm-reaper ./cmd/csm-reaper
	go build -ldflags="$(LDFLAGS_AGM)" -o bin/agm-mcp-server ./cmd/agm-mcp-server
	go build -o bin/csm-attach-wrapper ./cmd/csm-attach-wrapper
	go build -o bin/csm-agent-wrapper ./cmd/csm-agent-wrapper

test:
	go test -v -cover ./...

test-bdd:
	go test -v ./test/e2e

lint:
	go fmt ./...
	go vet ./...

install: build
	@# Clean up old binaries from incorrect locations to prevent confusion
	@rm -f $(HOME)/bin/csm $(HOME)/.local/bin/csm 2>/dev/null || true
	@rm -f $(HOME)/bin/csm-reaper $(HOME)/.local/bin/csm-reaper 2>/dev/null || true
	@rm -f $(HOME)/bin/agm-mcp-server $(HOME)/.local/bin/agm-mcp-server 2>/dev/null || true
	@rm -f $(HOME)/bin/csm-attach-wrapper $(HOME)/.local/bin/csm-attach-wrapper 2>/dev/null || true
	@rm -f $(HOME)/bin/csm-agent-wrapper $(HOME)/.local/bin/csm-agent-wrapper 2>/dev/null || true
	@# Install to standard Go binary location
	mkdir -p $(HOME)/go/bin
	cp bin/csm $(HOME)/go/bin/csm
	cp bin/csm-reaper $(HOME)/go/bin/csm-reaper
	cp bin/agm-mcp-server $(HOME)/go/bin/agm-mcp-server
	cp bin/csm-attach-wrapper $(HOME)/go/bin/csm-attach-wrapper
	cp bin/csm-agent-wrapper $(HOME)/go/bin/csm-agent-wrapper
	@echo "✓ csm binary installed to $(HOME)/go/bin/csm"
	@echo "✓ csm-reaper binary installed to $(HOME)/go/bin/csm-reaper"
	@echo "✓ agm-mcp-server binary installed to $(HOME)/go/bin/agm-mcp-server"
	@echo "✓ csm-attach-wrapper binary installed to $(HOME)/go/bin/csm-attach-wrapper"
	@echo "✓ csm-agent-wrapper binary installed to $(HOME)/go/bin/csm-agent-wrapper"
	@echo ""
	@echo "Cleaned up old binaries from:"
	@echo "  - $(HOME)/bin/csm (removed if existed)"
	@echo "  - $(HOME)/.local/bin/csm (removed if existed)"
	@echo "  - $(HOME)/bin/csm-reaper (removed if existed)"
	@echo "  - $(HOME)/.local/bin/csm-reaper (removed if existed)"
	@echo ""
	@echo "To install slash commands, add this repo as a plugin marketplace:"
	@echo "  /plugin marketplace add $(shell pwd)"
	@echo "  /plugin install csm-tools@ai-tools"
	@echo ""
	@echo "See PLUGIN-INSTALLATION.md for details"
	@echo ""
	@echo "Make sure $(HOME)/go/bin is in your PATH"

clean:
	rm -rf bin/
