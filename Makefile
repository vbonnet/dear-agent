# Root Makefile for dear-agent
#
# Targets:
#   lint-specs              Validate EARS requirements in SPEC.md files
#   preflight               Fast local CI-parity gates: vet + build + lint  (~25s)
#   preflight-tests         preflight + go test (no -race) — quick sanity
#   preflight-race          preflight + go test -race — catch data races before push
#   preflight-full          preflight + go test -race + govulncheck (full parity)
#   health-check            Run the codebase health auditor (cmd/repo-health)
#   install-preflight-hook  Install a git pre-push hook that runs preflight
#   install-post-merge-hook Install a post-merge hook that reaps merged worktrees
#   act-validate            Run full local CI validation via act (needs Docker)
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
#   build-write-guards      Build the PreToolUse fs/bash write-guard hooks
#   install-write-guards    Install the write-guard hooks into the hooks dir
#   build-bumblebee         Build the dear-agent-bumblebee Go binary
#   bumblebee-install       Install pinned, checksum-verified Bumblebee binary
#   bumblebee-scan          Run a one-shot Bumblebee endpoint scan
#   install-bumblebee-launchagent    Schedule the daily Bumblebee scan (macOS)
#   uninstall-bumblebee-launchagent  Remove the daily Bumblebee scan
#   build-jaeger-health     Build the Jaeger health-check CLI (cmd/jaeger-health)
#   install-jaeger-health   Install jaeger-health to ~/go/bin
#   build-bead-pr-guard     Build the bead-PR duplicate-guard CLI (cmd/bead-pr-guard)
#   install-bead-pr-guard   Install bead-pr-guard to ~/go/bin
#   build-safe-pr           Build safe-pr, the wayfinder-traced PR wrapper
#   install-safe-pr         Install safe-pr to ~/go/bin

.PHONY: lint-specs preflight preflight-tests preflight-race preflight-full health-check install-preflight-hook install-post-merge-hook act-validate act-lint act-test install-hooks test test-affected test-affected-print test-shell build-configure-settings install-configure-settings build-safe-push install-safe-push build-safe-pr install-safe-pr build-write-guards install-write-guards uninstall codegraph codegraph-all codegraph-install sync-main deepsec-incremental deepsec-staged install-deepsec-hook uninstall-deepsec-hook build-bumblebee bumblebee-install bumblebee-scan install-bumblebee-launchagent uninstall-bumblebee-launchagent structural-health structural-health-baseline build-src-recovery install-src-recovery build-jaeger-health install-jaeger-health build-bead-pr-sync install-bead-pr-sync build-bead-pr-guard install-bead-pr-guard

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

# preflight + `go test -race -count=1`. Same as CI's race detector pass but
# skips govulncheck. Use this before pushing any package that uses
# package-level mutable state (var func seams, global registries, shared
# maps) — races only show up with the race detector enabled, and CI runs
# with -race while local `make preflight-tests` does not.
preflight-race:
	@./scripts/preflight.sh --race

# Full CI parity: preflight + `go test -race -count=1` + govulncheck with
# the same allowlist as ci.yml. Slower but gives the highest confidence
# before pushing.
preflight-full:
	@./scripts/preflight.sh --full

# Build and run the codebase health auditor against this repo. Prints a
# markdown summary and exits 0 (healthy) / 1 (degraded) / 2 (critical).
# Pass ARGS to forward flags, e.g. `make health-check ARGS=--coverage` or
# `make health-check ARGS="--json-out health.json --md-out health.md"`.
# The scheduled .github/workflows/health-check.yml runs the same binary.
health-check:
	@mkdir -p build
	@GOWORK=off go build -o build/repo-health ./cmd/repo-health
	@./build/repo-health --root . $(ARGS)

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

# Install the post-merge worktree-sweep trigger. After a PR lands on the
# default branch locally (e.g. `git pull` on main), it kicks off the canonical
# fail-safe reaper `agm worktree sweep --execute`. The installer honours
# core.hooksPath and refuses to clobber a chezmoi-managed hooks dir — see
# cmd/install-post-merge-hook for the resolution and safety logic.
install-post-merge-hook:
	@go run ./cmd/install-post-merge-hook

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
	@go run ./cmd/test-affected --base=origin/main --tags=integration --run

# Print the affected package list without running anything. Useful for
# debugging "why did CI run/skip this suite?"
test-affected-print:
	@go run ./cmd/test-affected --base=origin/main --tags=integration

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

# Build safe-push: a git-push wrapper that resets the credential helper chain
# to gh-only (never osxkeychain, which can hang on a headless GUI prompt) and
# never force-pushes. See internal/safegit and docs/retros/2026-06-08-git-push-credential-hang.md.
build-safe-push:
	@echo "Building safe-push..."
	go build $(GOFLAGS) -o bin/safe-push ./cmd/safe-push/
	@echo "Built: bin/safe-push"

# Install safe-push to GOPATH/bin so it is on PATH for every agent session.
install-safe-push: build-safe-push
	cp bin/safe-push $(HOME)/go/bin/
	@echo "Installed: $(HOME)/go/bin/safe-push"

# Build safe-pr: the one sanctioned path for opening/closing GitHub PRs from
# agent sessions. It requires an active wayfinder session (WAYFINDER-STATUS.md
# status: in_progress) and stamps the session trace into the PR body/comment;
# .claude/hooks/pretool-pr-guard denies the raw gh verbs and points here.
# See internal/safepr and CLAUDE.md §PR Lifecycle (bead ce-p17s).
build-safe-pr:
	@echo "Building safe-pr..."
	go build $(GOFLAGS) -o bin/safe-pr ./cmd/safe-pr/
	@echo "Built: bin/safe-pr"

# Install safe-pr to GOPATH/bin so it is on PATH for every agent session.
# `Bash(safe-pr:*)` is allow-listed in .claude/settings.json — its safety is
# guaranteed by construction (CLAUDE.md principle 9).
install-safe-pr: build-safe-pr
	cp bin/safe-pr $(HOME)/go/bin/
	@echo "Installed: $(HOME)/go/bin/safe-pr"

# Build src-recovery: the one sanctioned writer to ~/src/**. It restores a
# golden checkout to a clean, current default branch via exactly stash ->
# checkout default -> pull --ff-only, takes no pass-through git args, and
# refuses every other git verb by construction. See internal/safesrc and
# docs/retros/2026-06-11-src-violations-and-burndown.md.
build-src-recovery:
	@echo "Building src-recovery..."
	go build $(GOFLAGS) -o bin/src-recovery ./cmd/src-recovery/
	@echo "Built: bin/src-recovery"

# Install src-recovery to GOPATH/bin so it is on PATH for every agent session.
# Allow-list `Bash(src-recovery *)` in chezmoi (dot_claude/private_settings.json.tmpl)
# alongside chezmoi-deploy and safe-push — its safety is guaranteed by
# construction, so it needs no per-invocation approval (CLAUDE.md principle 9).
install-src-recovery: build-src-recovery
	cp bin/src-recovery $(HOME)/go/bin/
	@echo "Installed: $(HOME)/go/bin/src-recovery"

# Build the Jaeger health-check CLI. Reports whether Jaeger at localhost:16686
# is alive and receiving traces. Exit codes: 0 healthy, 1 degraded (no recent
# traces), 2 down. Designed for use as a scheduled-task probe.
# Usage: jaeger-health [--url http://localhost:16686] [--lookback 1h] [--json]
build-jaeger-health:
	@echo "Building jaeger-health..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/jaeger-health ./cmd/jaeger-health/
	@echo "Built: bin/jaeger-health"

install-jaeger-health: build-jaeger-health
	cp bin/jaeger-health $(HOME)/go/bin/
	@echo "Installed: $(HOME)/go/bin/jaeger-health"

# Build bead-pr-sync: reconciles bead CLOSED status against GitHub PR merge
# state. Finds beads closed while their PR is still open (DoD violations) and
# reopens them to in_progress. See cmd/bead-pr-sync and ce-vqju.
build-bead-pr-sync:
	@echo "Building bead-pr-sync..."
	go build $(GOFLAGS) -o bin/bead-pr-sync ./cmd/bead-pr-sync/
	@echo "Built: bin/bead-pr-sync"

install-bead-pr-sync: build-bead-pr-sync
	cp bin/bead-pr-sync $(HOME)/go/bin/
	@echo "Installed: $(HOME)/go/bin/bead-pr-sync"

# Checks for an existing open PR claiming a bead before creating a new one.
# Usage: bead-pr-guard --bead <id> [--repo owner/name]
build-bead-pr-guard:
	@echo "Building bead-pr-guard..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/bead-pr-guard ./cmd/bead-pr-guard/
	@echo "Built: bin/bead-pr-guard"

install-bead-pr-guard: build-bead-pr-guard
	cp bin/bead-pr-guard $(HOME)/go/bin/
	@echo "Installed: $(HOME)/go/bin/bead-pr-guard"

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

# Run the structural-health scans and diff against the checked-in baseline.
# Fails only on regressions. Mirrors the Structural Health CI job.
structural-health:
	@go run ./cmd/structural-health

# Re-snapshot the structural-health baseline after fixing findings. Commit
# the resulting .structural-health-baseline.json to tighten the ratchet.
structural-health-baseline:
	@go run ./cmd/structural-health --update-baseline

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
