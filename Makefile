# Root Makefile for dear-agent
#
# Targets:
#   act-validate    Run full local CI validation via act
#   act-lint        Run lint job via act
#   act-test        Run test job via act
#   install-hooks   Install git pre-push hook for act validation
#   codegraph       Build a tree-sitter knowledge graph for this repo
#   codegraph-all   Build graphs for dear-agent and brain-v2

.PHONY: act-validate act-lint act-test install-hooks test-shell build-configure-settings uninstall codegraph codegraph-all codegraph-install

# Run full local CI validation via act
act-validate: act-lint act-test
	@echo "All act jobs passed."

# Run lint job via act
act-lint:
	@echo "[act] running lint job..."
	act -j lint -e .github/act/event-push.json

# Run test job via act
act-test:
	@echo "[act] running unit-tests job..."
	act -j unit-tests -e .github/act/event-push.json

# Run Bats shell tests
test-shell:
	@echo "Running Bats shell tests..."
	@if ! command -v bats >/dev/null 2>&1; then \
		echo "Error: bats not found. Install with: sudo apt-get install bats"; \
		exit 1; \
	fi
	@if [ ! -d tests/test_helper/bats-support ]; then \
		echo "Installing Bats helpers..."; \
		mkdir -p tests/test_helper; \
		git clone --depth 1 https://github.com/bats-core/bats-support.git tests/test_helper/bats-support; \
		git clone --depth 1 https://github.com/bats-core/bats-assert.git tests/test_helper/bats-assert; \
		git clone --depth 1 https://github.com/bats-core/bats-file.git tests/test_helper/bats-file; \
	fi
	bats tests/bats/

# Install git pre-push hook that runs act before push
# Uses the prepush-act-validator binary from the engram repo
install-hooks:
	@echo "Installing git pre-push hook..."
	@HOOK_DIR=$$(git -C . rev-parse --git-dir)/hooks; \
	mkdir -p $$HOOK_DIR; \
	VALIDATOR=$$(command -v prepush-act-validator 2>/dev/null); \
	if [ -z "$$VALIDATOR" ]; then \
		echo "Error: prepush-act-validator not found in PATH"; \
		echo "Build it from engram repo: make -C <engram>/hooks build-prepush"; \
		exit 1; \
	fi; \
	printf '#!/bin/sh\nexec %s\n' "$$VALIDATOR" > $$HOOK_DIR/pre-push; \
	chmod +x $$HOOK_DIR/pre-push; \
	echo "Installed: $$HOOK_DIR/pre-push -> $$VALIDATOR"

# Build configure-claude-settings tool
build-configure-settings:
	@echo "Building configure-claude-settings..."
	go build $(GOFLAGS) -o bin/configure-claude-settings ./cmd/configure-claude-settings/
	@echo "Built: bin/configure-claude-settings"

# Install configure-claude-settings to GOPATH/bin
install-configure-settings: build-configure-settings
	cp bin/configure-claude-settings $(HOME)/go/bin/
	@echo "Installed: $(HOME)/go/bin/configure-claude-settings"

# Uninstall AGM components
uninstall:
	@./scripts/uninstall.sh

# Build a tree-sitter knowledge graph (graphify) for this repo.
# Output lands in ~/.local/share/codegraph/<repo>/. See docs/codegraph.md.
codegraph:
	@./scripts/codegraph

# Build graphs for dear-agent and brain-v2 (the two codebases the team queries).
codegraph-all:
	@./scripts/codegraph $(CURDIR)
	@if [ -d $$HOME/src/brain-v2 ]; then \
		./scripts/codegraph $$HOME/src/brain-v2; \
	else \
		echo "skip: $$HOME/src/brain-v2 not present"; \
	fi

# Bootstrap the graphify venv at ~/.local/venvs/graphify.
codegraph-install:
	@./scripts/codegraph install
