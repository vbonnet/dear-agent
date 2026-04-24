.PHONY: build test lint install clean test-unit test-coverage coverage test-e2e-docker test-all install-skills init test-hooks test-bdd verify-contracts verify-specs

# Version information
VERSION ?= 2.0.0-dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILT_BY ?= $(shell whoami)

# Ldflags for version injection
LDFLAGS_AGM_COMPAT = -s -w \
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
	go build -ldflags="$(LDFLAGS_REAPER)" -o bin/agm-reaper ./cmd/agm-reaper
	go build -ldflags="$(LDFLAGS_AGM)" -o bin/agm-mcp-server ./cmd/agm-mcp-server

test:
	CGO_ENABLED=1 go test -tags="fts5" -v -cover ./...

lint:
	golangci-lint run ./...

install: build
	@# Clean up old binaries from incorrect locations to prevent confusion
	@rm -f $(HOME)/bin/agm $(HOME)/.local/bin/agm 2>/dev/null || true
	@rm -f $(HOME)/bin/csm $(HOME)/.local/bin/csm 2>/dev/null || true  # Remove legacy csm binaries
	@rm -f $(HOME)/bin/agm-reaper $(HOME)/.local/bin/agm-reaper 2>/dev/null || true
	@rm -f $(HOME)/bin/agm-mcp-server $(HOME)/.local/bin/agm-mcp-server 2>/dev/null || true
	@# Install to standard Go binary location (using install for atomic replacement)
	mkdir -p $(HOME)/go/bin
	install -m 755 bin/agm $(HOME)/go/bin/agm
	install -m 755 bin/agm-reaper $(HOME)/go/bin/agm-reaper
	install -m 755 bin/agm-mcp-server $(HOME)/go/bin/agm-mcp-server
	@echo "✓ agm binary installed to $(HOME)/go/bin/agm"
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
	@echo "Cleaned up old binaries from \$$(HOME)/bin/ and \$$(HOME)/.local/bin/"
	@echo ""
	@echo "To enable tab completion in current shell:"
	@echo "  source <(agm completion zsh)"
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

test-bdd: ## Run BDD feature tests (godog)
	@echo "Running BDD feature tests..."
	go test -v -run TestFeatures ./test/bdd/...
	@echo ""
	@echo "✓ BDD tests completed"

verify-contracts: ## Run contract drift detection against SPECs
	@echo "Running contract drift detection..."
	go test -v -run TestContractDrift ./test/bdd/...
	@echo ""
	@echo "✓ Contract drift check completed"

verify-specs: ## Run all SPEC verification: contract drift + BDD features + invariant tests
	@echo "=== SPEC Verification Suite ==="
	@echo ""
	@echo "--- Contract Drift Detection ---"
	go test -v -run TestContractDrift ./test/bdd/...
	@echo ""
	@echo "--- SPEC Invariant Tests ---"
	go test -v -run TestSPECInvariants ./test/bdd/...
	@echo ""
	@echo "--- BDD Feature Tests ---"
	go test -v -run TestFeatures ./test/bdd/...
	@echo ""
	@echo "✓ All SPEC verification passed"

test-all: test-unit test-bdd test-e2e-docker
	@echo ""
	@echo "✓ All tests passed"

.PHONY: install-skills
install-skills: ## Install Claude Code skills
	@echo "Installing AGM skills to $(HOME)/.claude/skills/agm/"
	@mkdir -p $(HOME)/.claude/skills/agm
	@cp skills/*.sh $(HOME)/.claude/skills/agm/
	@chmod +x $(HOME)/.claude/skills/agm/*.sh
	@cp skills/README.md $(HOME)/.claude/skills/agm/
	@echo "✓ Skills installed successfully"
	@echo ""
	@echo "Available skills:"
	@echo "  /agm:new      - Create new session"
	@echo "  /agm:send     - Send messages"
	@echo "  /agm:status   - Check session status"
	@echo "  /agm:resume   - Resume session"

.PHONY: plugin-hashes
plugin-hashes: ## Generate content hashes for AGM plugin skills
	@for file in agm-plugin/commands/*.md; do \
		body=$$(awk 'BEGIN{p=0} /^---$$/{p++; next} p>=2' "$$file"); \
		hash=$$(printf '%s' "$$body" | sha256sum | awk '{print $$1}'); \
		current=$$(grep '^content-hash:' "$$file" | awk '{print $$2}'); \
		if [ "$$current" != "$$hash" ]; then \
			sed -i "s/^content-hash: .*/content-hash: $$hash/" "$$file"; \
			echo "  updated $$file -> $$hash"; \
		else \
			echo "  unchanged $$file"; \
		fi; \
	done

.PHONY: plugin-verify-hashes
plugin-verify-hashes: ## Verify content hashes match current skill content
	@failed=0; \
	for file in agm-plugin/commands/*.md; do \
		current=$$(grep '^content-hash:' "$$file" | awk '{print $$2}'); \
		body=$$(awk 'BEGIN{p=0} /^---$$/{p++; next} p>=2' "$$file"); \
		expected=$$(printf '%s' "$$body" | sha256sum | awk '{print $$1}'); \
		if [ "$$current" = "PLACEHOLDER" ]; then \
			echo "  FAIL $$file: still has PLACEHOLDER hash"; \
			failed=$$((failed + 1)); \
		elif [ "$$current" != "$$expected" ]; then \
			echo "  FAIL $$file: hash mismatch"; \
			failed=$$((failed + 1)); \
		else \
			echo "  OK $$file"; \
		fi; \
	done; \
	if [ $$failed -gt 0 ]; then \
		echo ""; \
		echo "$$failed file(s) have stale hashes. Run 'make plugin-hashes' to fix."; \
		exit 1; \
	fi

init: ## Configure git hooks (test enforcement + post-commit auto-build)
	git config core.hooksPath claude-session-manager/.githooks
	@echo "✓ Git hooks configured (core.hooksPath = claude-session-manager/.githooks)"
	@echo "  Post-commit hook will auto-rebuild AGM binary when source changes."
	@echo "  Override with: AGM_SKIP_TEST_GATE=1 git commit ..."

test-hooks: ## Run enforcement gate hook tests
	claude-session-manager/.githooks/test-enforcement-gates.sh
