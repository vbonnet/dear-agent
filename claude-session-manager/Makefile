.PHONY: build test lint install clean test-unit test-coverage coverage test-e2e-docker test-all

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
	go build -ldflags="$(LDFLAGS_REAPER)" -o bin/agm-reaper ./cmd/agm-reaper
	go build -ldflags="$(LDFLAGS_AGM)" -o bin/agm-mcp-server ./cmd/agm-mcp-server

test:
	CGO_ENABLED=1 go test -tags="fts5" -v -cover ./...

lint:
	go fmt ./...
	go vet ./...

install: build
	@# Clean up old binaries from incorrect locations to prevent confusion
	@rm -f $(HOME)/bin/agm $(HOME)/.local/bin/agm 2>/dev/null || true
	@rm -f $(HOME)/bin/csm $(HOME)/.local/bin/csm 2>/dev/null || true
	@rm -f $(HOME)/bin/agm-reaper $(HOME)/.local/bin/agm-reaper 2>/dev/null || true
	@rm -f $(HOME)/bin/agm-mcp-server $(HOME)/.local/bin/agm-mcp-server 2>/dev/null || true
	@# Install to standard Go binary location (using install for atomic replacement)
	mkdir -p $(HOME)/go/bin
	install -m 755 bin/agm $(HOME)/go/bin/agm
	install -m 755 bin/csm $(HOME)/go/bin/csm
	install -m 755 bin/agm-reaper $(HOME)/go/bin/agm-reaper
	install -m 755 bin/agm-mcp-server $(HOME)/go/bin/agm-mcp-server
	@echo "✓ agm binary installed to $(HOME)/go/bin/agm"
	@echo "✓ csm wrapper binary installed to $(HOME)/go/bin/csm (forwards to agm)"
	@echo "✓ agm-reaper binary installed to $(HOME)/go/bin/agm-reaper"
	@echo "✓ agm-mcp-server binary installed to $(HOME)/go/bin/agm-mcp-server"
	@echo ""
	@# Install SessionStart hook (auto-discovered from $(HOME)/.claude/hooks/session-start/)
	@mkdir -p $(HOME)/.claude/hooks/session-start
	@install -m 755 docs/hooks/session-start-agm.sh $(HOME)/.claude/hooks/session-start/agm-ready-signal
	@echo "✓ SessionStart hook installed to $(HOME)/.claude/hooks/session-start/agm-ready-signal"
	@echo ""
	@# Setup bash completion
	@if [ -f $(HOME)/.bashrc ]; then \
		if ! grep -q "agm completion bash" $(HOME)/.bashrc; then \
			echo "" >> $(HOME)/.bashrc; \
			echo "# AGM (Agent Gateway Manager) bash completion" >> $(HOME)/.bashrc; \
			echo "if command -v agm &> /dev/null; then" >> $(HOME)/.bashrc; \
			echo "    source <(agm completion bash)" >> $(HOME)/.bashrc; \
			echo "fi" >> $(HOME)/.bashrc; \
			echo "✓ Added agm completion to $(HOME)/.bashrc"; \
		else \
			echo "✓ agm completion already in $(HOME)/.bashrc"; \
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
	@echo "  /plugin install agm@ai-tools"
	@echo ""
	@echo "See PLUGIN-INSTALLATION.md for details"
	@echo ""
	@echo "Make sure $(HOME)/go/bin is in your PATH"

clean:
	rm -rf bin/

# Test targets
test-unit:
	CGO_ENABLED=1 go test -tags="fts5" -v -cover ./...

test-coverage:
	CGO_ENABLED=1 go test -tags="fts5" -v -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

coverage: test-coverage

test-e2e-docker: build
	@echo "Running Docker E2E tests for reaper..."
	@echo ""
	@# Create test-results directory if it doesn't exist
	@mkdir -p test/e2e/docker/test-results
	@# Run Docker Compose tests
	@cd test/e2e/docker && docker compose up --build --abort-on-container-exit --exit-code-from reaper-e2e-test
	@# Cleanup
	@cd test/e2e/docker && docker compose down
	@echo ""
	@echo "✓ Docker E2E tests completed"

test-all: test-unit test-e2e-docker
	@echo ""
	@echo "✓ All tests passed"
