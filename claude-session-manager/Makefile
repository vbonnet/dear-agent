.PHONY: build test test-bdd lint install clean

# Version information
VERSION ?= 2.0.0-dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILT_BY ?= $(shell whoami)

# Ldflags for version injection
LDFLAGS_CSM = -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X main.BuiltBy=$(BUILT_BY)

LDFLAGS_REAPER = -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X main.BuiltBy=$(BUILT_BY)

LDFLAGS_AGM = -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X main.BuiltBy=$(BUILT_BY)

build:
	go build -ldflags="$(LDFLAGS_AGM)" -o bin/agm ./cmd/agm
	go build -o bin/csm ./cmd/csm
	go build -ldflags="$(LDFLAGS_AGM)" -o bin/agm-mcp-server ./cmd/agm-mcp-server

test:
	go test -v -cover ./...

test-bdd:
	go test -v ./test/e2e

lint:
	go fmt ./...
	go vet ./...

install: build
	@# Clean up old binaries from incorrect locations to prevent confusion
	@rm -f $(HOME)/bin/agm $(HOME)/.local/bin/agm 2>/dev/null || true
	@rm -f $(HOME)/bin/csm $(HOME)/.local/bin/csm 2>/dev/null || true
	@rm -f $(HOME)/bin/agm-mcp-server $(HOME)/.local/bin/agm-mcp-server 2>/dev/null || true
	@# Install to standard Go binary location (using install for atomic replacement)
	mkdir -p $(HOME)/go/bin
	install -m 755 bin/agm $(HOME)/go/bin/agm
	install -m 755 bin/csm $(HOME)/go/bin/csm
	install -m 755 bin/agm-mcp-server $(HOME)/go/bin/agm-mcp-server
	@echo "✓ agm binary installed to $(HOME)/go/bin/agm"
	@echo "✓ csm wrapper binary installed to $(HOME)/go/bin/csm (forwards to agm)"
	@echo "✓ agm-mcp-server binary installed to $(HOME)/go/bin/agm-mcp-server"
	@echo ""
	@# Setup bash completion
	@if [ -f $(HOME)/.bashrc ]; then \
		if ! grep -q "agm completion bash" $(HOME)/.bashrc; then \
			echo "" >> $(HOME)/.bashrc; \
			echo "# AGM (Agent Gateway Manager) bash completion" >> $(HOME)/.bashrc; \
			echo "if command -v agm &> /dev/null; then" >> $(HOME)/.bashrc; \
			echo "    source <(agm completion bash)" >> $(HOME)/.bashrc; \
			echo "fi" >> $(HOME)/.bashrc; \
			echo "✓ Added agm completion to ~/.bashrc"; \
		else \
			echo "✓ agm completion already in ~/.bashrc"; \
		fi; \
	fi
	@echo ""
	@echo "⚠️  csm has been renamed to agm"
	@echo "  - Use 'agm' for new sessions (recommended)"
	@echo "  - 'csm' wrapper provided for compatibility (prints warning, forwards to agm)"
	@echo ""
	@echo "Cleaned up old binaries from:"
	@echo "  - $(HOME)/bin/agm (removed if existed)"
	@echo "  - $(HOME)/bin/csm (removed if existed)"
	@echo "  - $(HOME)/.local/bin/agm (removed if existed)"
	@echo "  - $(HOME)/.local/bin/csm (removed if existed)"
	@echo ""
	@echo "To enable tab completion in current shell:"
	@echo "  source <(agm completion bash)"
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
