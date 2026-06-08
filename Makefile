# Root Makefile for dear-agent
#
# Targets:
#   lint-specs              Validate EARS requirements in SPEC.md files
#   preflight               Fast local CI-parity gates: vet + build + lint  (~25s)
#   preflight-tests         preflight + go test (no -race) — quick sanity
#   preflight-full          preflight + go test -race + govulncheck (full parity)
#   install-preflight-hook  Install a git pre-push hook that runs preflight
#   act-validate            Run full local CI validation via act (needs Docker)
#   act-lint                Run lint job via act
#   act-test                Run test job via act
#   install-hooks           Install git pre-push hook for act validation
#   codegraph               Build a tree-sitter knowledge graph for this repo
#   codegraph-all           Build graphs for dear-agent and brain-v2
#   sync-main               Stash, fetch, rebase onto origin/main, then pop
#   deepsec-incremental     Scan files changed since origin/main with deepsec
#   deepsec-staged          Scan staged files only with deepsec
#   install-deepsec-hook    Install pre-push hook for incremental deepsec scans
#   uninstall-deepsec-hook  Remove the deepsec pre-push hook
#   build-write-guards      Build the PreToolUse fs/bash write-guard hooks
#   install-write-guards    Install the write-guard hooks into the hooks dir
#   build-bumblebee         Build the dear-agent-bumblebee Go binary
#   bumblebee-install       Install pinned, checksum-verified Bumblebee binary
#   bumblebee-scan          Run a one-shot Bumblebee endpoint scan
#   install-bumblebee-launchagent    Schedule the daily Bumblebee scan (macOS)
#   uninstall-bumblebee-launchagent  Remove the daily Bumblebee scan

.PHONY: lint-specs preflight preflight-tests preflight-full install-preflight-hook act-validate act-lint act-test install-hooks test-shell build-configure-settings install-configure-settings build-write-guards install-write-guards uninstall codegraph codegraph-all codegraph-install sync-main deepsec-incremental deepsec-staged install-deepsec-hook uninstall-deepsec-hook build-bumblebee bumblebee-install bumblebee-scan install-bumblebee-launchagent uninstall-bumblebee-launchagent

# Validate EARS-formatted requirements in SPEC.md files using the same
# deterministic linter the wayfinder D4/SPEC phase gate uses (cmd/ears-lint).
# Scans the whole repo by default; override PATHS to narrow the scope and set
# STRICT=1 to fail on any non-conforming requirement (not just files with zero
# valid requirements). Examples:
#   make lint-specs
#   make lint-specs PATHS=internal/sandbox/SPEC.md STRICT=1
lint-specs:
	@go run ./cmd/ears-lint $(if $(STRICT),--strict) $(if $(PATHS),$(PATHS),.)

# Fast local CI-parity gates. Runs the same go vet / go build / golangci-lint
# CI does, no Docker needed. Catches ~all lint failures in ~25s on a warm
# build cache. See docs/retros/2026-05-27-ci-shift-left.md for the rationale
# (CI on GitHub is not part of the inner dev loop).
preflight:
	@./scripts/preflight.sh --fast

# preflight + `go test` without -race. Faster than full CI parity but still
# catches behaviour regressions a lint-only sweep misses.
preflight-tests:
	@./scripts/preflight.sh --tests

# Full CI parity: preflight + `go test -race -count=1` + govulncheck with
# the same allowlist as ci.yml. Slower but gives the highest confidence
# before pushing.
preflight-full:
	@./scripts/preflight.sh --full

# Install a git pre-push hook that runs `make preflight`. Pushing to a PR
# branch will then fail-fast before the GitHub round-trip if lint/build/vet
# is broken. Does NOT replace CI — only shifts left. Refuses to overwrite
# an existing hook (deepsec, husky, etc.) — merge manually if you have one.
install-preflight-hook:
	@HOOK="$$(git rev-parse --git-path hooks/pre-push)"; \
	if [ -e "$$HOOK" ]; then \
		echo "Error: a pre-push hook already exists at $$HOOK"; \
		echo "Merge 'exec make preflight' into it manually, or remove it first."; \
		exit 1; \
	fi; \
	printf '#!/bin/sh\nexec make preflight\n' > "$$HOOK"; \
	chmod +x "$$HOOK"; \
	echo "Installed: $$HOOK -> make preflight"

# Run full local CI validation via act. Requires Docker + act installed.
# Prefer `make preflight-full` for the same gates without containerisation.
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

# Build the PreToolUse filesystem write-guard hooks. These enforce the
# worktree-only write policy (see internal/fsguard): pretool-fs-write-guard
# gates Edit/Write/MultiEdit, pretool-bash-write-guard gates Bash. They are
# the Go replacements for the lost ai-tools Python stopgaps.
build-write-guards:
	@echo "Building pretool-fs-write-guard, pretool-bash-write-guard..."
	go build $(GOFLAGS) -o bin/pretool-fs-write-guard ./cmd/pretool-fs-write-guard/
	go build $(GOFLAGS) -o bin/pretool-bash-write-guard ./cmd/pretool-bash-write-guard/
	@echo "Built: bin/pretool-fs-write-guard bin/pretool-bash-write-guard"

# Install the write-guard hooks where settings.json references them
# (~/.config/claude-code/hooks). Override the dir with HOOKS_DIR=/path.
HOOKS_DIR ?= $(HOME)/.config/claude-code/hooks
install-write-guards: build-write-guards
	@mkdir -p $(HOOKS_DIR)
	cp bin/pretool-fs-write-guard bin/pretool-bash-write-guard $(HOOKS_DIR)/
	@echo "Installed: $(HOOKS_DIR)/pretool-fs-write-guard $(HOOKS_DIR)/pretool-bash-write-guard"

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

# Build the bumblebee installer + scan-wrapper Go binary. Drops into
# bin/dear-agent-bumblebee; the LaunchAgent points at the installed copy
# (see install-bumblebee-launchagent below), so a `go install`'d binary on
# PATH is preferable for the scheduled case.
build-bumblebee:
	@echo "Building dear-agent-bumblebee..."
	go build $(GOFLAGS) -o bin/dear-agent-bumblebee ./cmd/dear-agent-bumblebee/
	@echo "Built: bin/dear-agent-bumblebee"

# Install a pinned, checksum-verified Bumblebee binary into ~/.local/bin
# (override with BUMBLEBEE_PREFIX=/path). Verifies SHA-256 before extracting
# the tarball — see ADR-027 and cmd/dear-agent-bumblebee/install.go.
bumblebee-install: build-bumblebee
	@./bin/dear-agent-bumblebee install $(if $(BUMBLEBEE_PREFIX),--prefix $(BUMBLEBEE_PREFIX),)

# Run a one-shot Bumblebee endpoint scan. NDJSON output lands in the per-user
# data dir; the wrapper prints a one-line summary. Honours BUMBLEBEE_BIN
# (binary override) and BUMBLEBEE_CATALOG (exposure catalog).
bumblebee-scan: build-bumblebee
	@./bin/dear-agent-bumblebee scan

# Install the daily Bumblebee LaunchAgent (macOS, per-user). Runs at 04:00
# local. See docs/bumblebee.md and ADR-027. The LaunchAgent invokes the
# installed dear-agent-bumblebee binary, so this target installs into
# $HOME/.local/bin first so the plist references a stable path.
install-bumblebee-launchagent: build-bumblebee
	@mkdir -p $(HOME)/.local/bin
	@install -m 0755 bin/dear-agent-bumblebee $(HOME)/.local/bin/dear-agent-bumblebee
	@$(HOME)/.local/bin/dear-agent-bumblebee install-launchagent

uninstall-bumblebee-launchagent: build-bumblebee
	@./bin/dear-agent-bumblebee install-launchagent --uninstall
