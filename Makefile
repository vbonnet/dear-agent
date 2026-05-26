# Root Makefile for dear-agent
#
# Targets:
#   act-validate            Run full local CI validation via act
#   act-lint                Run lint job via act
#   act-test                Run test job via act
#   install-hooks           Install git pre-push hook for act validation
#   test                    Run the full unit-test suite (go test ./...)
#   test-affected           Run only the integration tests affected by
#                           the current diff vs. origin/main
#   test-affected-print     Print the affected package list (no run)
#   codegraph               Build a tree-sitter knowledge graph for this repo
#   codegraph-all           Build graphs for dear-agent and brain-v2
#   sync-main               Stash, fetch, rebase onto origin/main, then pop
#   deepsec-incremental     Scan files changed since origin/main with deepsec
#   deepsec-staged          Scan staged files only with deepsec
#   install-deepsec-hook    Install pre-push hook for incremental deepsec scans
#   uninstall-deepsec-hook  Remove the deepsec pre-push hook

.PHONY: act-validate act-lint act-test install-hooks test test-affected test-affected-print test-shell build-configure-settings uninstall codegraph codegraph-all codegraph-install sync-main deepsec-incremental deepsec-staged install-deepsec-hook uninstall-deepsec-hook

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

# Run the full Go test suite. Mirrors what CI's "Build & Test" job does
# locally so a green `make test` is the same answer as a green CI.
test:
	go test -race -count=1 ./...

# Run only the integration tests whose packages (or their transitive
# dependencies) changed vs. origin/main. See cmd/test-affected and
# docs/adr/ADR-024 for the algorithm and trust boundaries.
#
# Safety nets baked into the selector: go.mod / go.sum / Makefile /
# .github/workflows / the selector itself fall back to a full run, so
# this target is safe to default to locally before pushing.
test-affected:
	@./scripts/test-affected.sh

# Print the affected package list without running anything. Useful for
# debugging "why did CI run/skip this suite?"
test-affected-print:
	@./scripts/test-affected.sh --dry-run

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

# Atomic stash / fetch / rebase / pop onto origin/main. Defaults to this
# repo; pass REPO=<path> to target a different working copy.
sync-main:
	@./scripts/git-sync-main.sh $(if $(REPO),$(REPO),$(CURDIR))

# Run deepsec on files changed since origin/main. Free locally — uses the
# `claude` CLI subscription if you have it logged in. See docs/deepsec.md.
deepsec-incremental:
	@./scripts/deepsec-incremental.sh

# Run deepsec on staged files only (use as a manual pre-commit check).
deepsec-staged:
	@./scripts/deepsec-incremental.sh --staged

# Install a pre-push hook that runs deepsec on the push delta. Soft-fail
# by default (warns, doesn't block). Use STRICT=1 to block pushes on
# findings; bypass once with DEEPSEC_SKIP=1 git push.
install-deepsec-hook:
	@./scripts/install-deepsec-hook.sh $(if $(STRICT),--strict,--soft)

uninstall-deepsec-hook:
	@./scripts/install-deepsec-hook.sh --uninstall
