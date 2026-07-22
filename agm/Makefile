include ../mk/install-go-bin.mk

.PHONY: build test lint install clean test-unit test-coverage coverage test-e2e-docker test-all install-skills init test-hooks test-bdd verify-contracts

# Version information
VERSION ?= 2.0.0-dev
GIT_COMMIT ?= $(shell commit=$$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown); if [ "$$commit" != unknown ] && [ -n "$$(git status --porcelain --untracked-files=no 2>/dev/null)" ]; then printf '%s-dirty' "$$commit"; else printf '%s' "$$commit"; fi)
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
	@# Install to the standard Go binary location. `install -m` was NOT atomic
	@# enough: like cp it rewrites the destination inode, so on macOS a rebuilt
	@# binary fails the cached code-signing check and is SIGKILLed before main()
	@# runs. install-go-bin stages, signs and renames instead (ce-77ip.8).
	mkdir -p $(HOME)/go/bin
	$(call install-go-bin,bin/agm)
	$(call install-go-bin,bin/agm-reaper)
	$(call install-go-bin,bin/agm-mcp-server)
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
	@# Redeploy Claude Code hooks so deployed version always matches the installed binary.
	@# Root cause of ce-710r: PR #456 shipped the orphan reaper but hooks were never
	@# redeployed, so the deployed hook predated the fix for 24h. Closes gap G1.
	$(HOME)/go/bin/agm admin install-hooks
	@echo "✓ Claude Code hooks redeployed (gap G1 closed: built == deployed)"
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

test-all: test-unit test-bdd test-e2e-docker
	@echo ""
	@echo "✓ All tests passed"

.PHONY: install-commands install-skills
install-commands: ## Install legacy global Claude command files
	@./install-commands.sh

install-skills: install-commands ## Compatibility alias for install-commands

.PHONY: plugin-hashes
plugin-hashes: ## Generate content hashes for AGM plugin skills
	@go run ./cmd/plugin-hash --dir agm-plugin/commands

.PHONY: plugin-verify-hashes
plugin-verify-hashes: ## Verify content hashes match current skill content
	@go run ./cmd/plugin-hash --dir agm-plugin/commands --check

init: ## Configure git hooks (test enforcement + post-commit auto-build)
	git config core.hooksPath claude-session-manager/.githooks
	@echo "✓ Git hooks configured (core.hooksPath = claude-session-manager/.githooks)"
	@echo "  Post-commit hook will auto-rebuild AGM binary when source changes."
	@echo "  Override with: AGM_SKIP_TEST_GATE=1 git commit ..."

test-hooks: ## Run enforcement gate hook tests
	claude-session-manager/.githooks/test-enforcement-gates.sh
