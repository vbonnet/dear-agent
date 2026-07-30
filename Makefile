# Root Makefile for dear-agent
#
# Go memory/CPU baseline (ce-1bhq). Load the checked-in envelope and export it
# so every build/test/run target — and any tool they shell out to — inherits a
# bounded GOMEMLIMIT/GOMAXPROCS/GOGC instead of growing unbounded. Override on
# the command line (e.g. `make test GOGC=200`) or per-daemon in its launch
# wrapper. `-include` so a missing file degrades to Go defaults, not a hard
# error. See env/go-baseline.env for the rationale behind each value.
-include env/go-baseline.env
export GOMEMLIMIT GOMAXPROCS GOGC

# Version stamping (ce-wy1q). Injected into every binary via -ldflags so that
# `<binary> --version` reports the actual build provenance.
# Override on the CLI: make build-safe-pr VERSION=1.2.3
VERSION    ?= dev
GIT_COMMIT ?= $(shell commit=$$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown); if [ "$$commit" != unknown ] && [ -n "$$(git status --porcelain --untracked-files=no 2>/dev/null)" ]; then printf '%s-dirty' "$$commit"; else printf '%s' "$$commit"; fi)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

PKG_VERSION := github.com/vbonnet/dear-agent/pkg/version
VERSION_LDFLAGS := \
	-X '$(PKG_VERSION).Version=$(VERSION)' \
	-X '$(PKG_VERSION).GitCommit=$(GIT_COMMIT)' \
	-X '$(PKG_VERSION).BuildDate=$(BUILD_DATE)' \
	-X '$(PKG_VERSION).BuiltBy=makefile'

# GOFLAGS is passed to every `go build $(GOFLAGS)` call in this Makefile.
# Setting it here injects version info into all binaries at once.
GOFLAGS ?= -ldflags "$(VERSION_LDFLAGS)"
#
# Targets:
#   lint-specs              Validate EARS requirements in SPEC.md files
#   lint-skills             Validate every tracked skill and command prompt
#   verify-surface-codegen  Regenerate ignored AGM surface artifacts and fail on drift
#   plugin-verify-hashes    Verify AGM plugin command and skill content hashes
#   preflight               Fast local CI-parity gates: vet + build + AI skills + lint (~25s)
#   preflight-tests         preflight + go test (no -race) — quick sanity
#   preflight-race          preflight + go test -race — catch data races before push
#   preflight-full          preflight + go test -race + govulncheck (full parity)
#   health-check            Run the codebase health auditor (cmd/repo-health)
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
#   build-codex-hook-json    Build the fixed JSON helper for attested Codex hooks
#   install-codex-hook-json  Operator-install the digest-bound JSON helper
#   build-bead-close-guard  Build the DoD enforcement gate for bead closure (cmd/bead-close-guard)
#   install-bead-close-guard Install bead-close-guard to ~/go/bin and the operator-owned Codex hook path
#   build-drift-check       Build the legacy deployment-drift detector (cmd/drift-check)
#   install-drift-check     Install drift-check to ~/go/bin
#   deploy-status           Manifest-driven drift audit (dear-deploy status: hooks/plists + binary version stamps)
#   drift-check             Alias for deploy-status (the manifest-driven replacement)
#   drift-check-legacy      Run the legacy hash-only detector (cmd/drift-check)
#   build-babysit-prs       Build babysit-prs: serial PR updater + merger
#   install-babysit-prs     Install babysit-prs to ~/go/bin
#   build-pr-linkify        Build pr-linkify: PR reference linkifier (cmd/pr-linkify)
#   install-pr-linkify      Install pr-linkify to ~/go/bin
#   build-fd-pressure       Build fd-pressure: FD/vnode/gopls pressure monitor
#   install-fd-pressure     Install fd-pressure to ~/go/bin
#   build-gopls-watchdog    Build gopls-watchdog: gopls alarm + auto-remediation (cmd/gopls-watchdog)
#   install-gopls-watchdog  Install gopls-watchdog to ~/go/bin
#   install-gopls-watchdog-launchagent   Stage the gopls-watchdog launch agent (2-min tick)
#   uninstall-gopls-watchdog-launchagent Remove the gopls-watchdog launch agent
#   install-sandbox-gc-launchagent   Stage the hourly sandbox GC launch agent (ce-uxju)
#   uninstall-sandbox-gc-launchagent Remove the sandbox GC launch agent
#   build-disk-watchdog     Build disk-watchdog: disk-free/inode alarm + worktree-sweep remediation (ce-6fel)
#   install-disk-watchdog   Install disk-watchdog to ~/go/bin
#   install-disk-watchdog-launchagent   Stage the disk-watchdog launch agent (5-min tick)
#   uninstall-disk-watchdog-launchagent Remove the disk-watchdog launch agent
#   build-override-ledger-helper        Build the fixed privileged Unix ledger append helper
#   install-override-ledger-helper      Operator-install the helper and exact sudoers rule (Unix)
#   install-override-audit-launchdaemon Install the macOS dangerous-override audit
#   uninstall-override-audit-launchdaemon Remove the macOS dangerous-override audit
#   install-override-audit-systemd      Operator-install the daily dangerous-override audit (Linux)
#   uninstall-override-audit-systemd    Remove the Linux dangerous-override audit
#   install-gobin-guard             Install the ~/go/bin SENSE+ESCALATE guard outside GOBIN (ce-24f1)
#   install-gobin-guard-launchagent Stage the gobin-guard launch agent (60-sec tick)
#   uninstall-gobin-guard-launchagent Remove the gobin-guard launch agent
#   build-vroom-dispatch    Build vroom-dispatch: VROOM supervisor mesh launcher
#   install-vroom-dispatch  Install vroom-dispatch to ~/go/bin
#   build-vroom-mesh        Build vroom-mesh: in-process 3-supervisor mesh harness with real adapters (ce-plf0)
#   install-vroom-mesh      Install vroom-mesh to ~/go/bin
#   build-agm-bus           Build agm-bus channel MCP adapter (TypeScript, npm run build)
#   build-vroom-prompt-gen  Build vroom-prompt-gen: orchestrator prompt-library refresher (ce-5z0o)
#   install-vroom-prompt-gen Install vroom-prompt-gen to ~/go/bin
#   build-resolve-review-threads  Build resolve-review-threads: GitHub PR thread resolver
#   install-resolve-review-threads Install resolve-review-threads to ~/go/bin
#   build-merge-audit       Build merge-audit: safe-merge P6 detection tier
#   install-merge-audit     Install merge-audit to ~/go/bin
#   install-token-refresher-launchagent   Schedule the OAuth token-refresher idle backstop (macOS, ce-cs3v)
#   uninstall-token-refresher-launchagent Remove the token-refresher launch agent
#   build-dear-deploy       Build dear-deploy: atomic host-artifact deployer (cmd/dear-deploy)
#   install-dear-deploy     Install dear-deploy to ~/go/bin
#   dear-deploy-sync        Deploy host artifacts from deploy/manifest.yaml (build guards first)
#   build-agm-job           Build agm-job: host-side job runner (ce-m3ya)
#   install-agm-job         Install agm-job to ~/go/bin
#   build-src-health        Build src-health: ~/src repo canary (ce-m3ya)
#   install-src-health      Install src-health to ~/go/bin
#   build-burndown-maint    Build burndown-maint: host-side bead-burndown maintenance (ce-cd14.2)
#   install-burndown-maint  Install burndown-maint to ~/go/bin
#   build-vroom-governor    Build vroom-governor: system load/RAM monitor that pauses/resumes spawns (ce-lxdo)
#   install-vroom-governor  Install vroom-governor to ~/go/bin
#   build-session-skill-extractor  Build session-skill-extractor: extract reusable SKILL candidates from sessions (ce-ouvr)
#   install-session-skill-extractor Install session-skill-extractor to ~/go/bin

.PHONY: lint-specs preflight preflight-tests preflight-race preflight-full health-check install-preflight-hook install-post-merge-hook build-routing-guard install-routing-guard-hook act-validate act-lint act-test install-hooks test test-affected test-affected-print test-shell build-configure-settings install-configure-settings build-safe-push install-safe-push build-safe-merge install-safe-merge build-safe-rebase install-safe-rebase build-safe-pr install-safe-pr build-write-guards install-write-guards uninstall codegraph codegraph-all codegraph-install sync-main deepsec-incremental deepsec-staged install-deepsec-hook uninstall-deepsec-hook build-bumblebee bumblebee-install bumblebee-scan install-bumblebee-launchagent uninstall-bumblebee-launchagent structural-health structural-health-baseline build-src-recovery install-src-recovery build-safe-unlock install-safe-unlock build-jaeger-health install-jaeger-health build-bead-pr-sync install-bead-pr-sync install-bead-pr-sync-launchagent uninstall-bead-pr-sync-launchagent build-bead-pr-guard install-bead-pr-guard build-codex-hook-json install-codex-hook-json build-bead-close-guard install-bead-close-guard build-babysit-prs install-babysit-prs build-pr-linkify install-pr-linkify build-mergeloop install-mergeloop install-mergeloop-launchagent uninstall-mergeloop-launchagent build-drift-check install-drift-check drift-check drift-check-legacy deploy-status build-fd-pressure install-fd-pressure build-gopls-watchdog install-gopls-watchdog install-gopls-watchdog-launchagent uninstall-gopls-watchdog-launchagent uninstall-sandbox-gc-launchagent install-sandbox-gc-launchagent build-disk-watchdog install-disk-watchdog install-disk-watchdog-launchagent uninstall-disk-watchdog-launchagent install-override-audit-launchdaemon uninstall-override-audit-launchdaemon install-override-audit-systemd uninstall-override-audit-systemd install-gobin-guard install-gobin-guard-launchagent uninstall-gobin-guard-launchagent build-vroom-dispatch install-vroom-dispatch build-vroom-mesh install-vroom-mesh build-agm-bus build-vroom-prompt-gen install-vroom-prompt-gen build-resolve-review-threads install-resolve-review-threads build-merge-audit install-merge-audit build-token-refresher install-token-refresher install-token-refresher-launchagent uninstall-token-refresher-launchagent build-dear-deploy install-dear-deploy dear-deploy-sync build-agm-job install-agm-job build-src-health install-src-health build-burndown-maint install-burndown-maint install-fd-limit-launchdaemon uninstall-fd-limit-launchdaemon build-otel-local install-otel-local otel-up build-vroom-governor install-vroom-governor build-agm install-agm build-agm-mcp-server install-agm-mcp-server build-engram-mcp install-engram-mcp
.PHONY: build-session-skill-extractor install-session-skill-extractor
.PHONY: lint-skills
.PHONY: lint-instructions
.PHONY: lint-adrs
.PHONY: lint-headers
.PHONY: build-override-ledger-helper install-override-ledger-helper

include mk/install-go-bin.mk

# Validate EARS-formatted requirements in SPEC.md files using the same
# deterministic linter the wayfinder D4/SPEC phase gate uses (cmd/ears-lint).
# Scans the whole repo by default; override PATHS to narrow the scope and set
# STRICT=1 to fail on any non-conforming requirement (not just files with zero
# valid requirements). Examples:
#   make lint-specs
#   make lint-specs PATHS=internal/sandbox/SPEC.md STRICT=1
lint-specs:
	@go run ./cmd/ears-lint $(if $(STRICT),--strict) $(if $(PATHS),$(PATHS),.)

# Validate the Git-tracked AI skill inventory through the same repository
# interface required CI uses. Hidden and nonstandard skill roots are included.
lint-skills:
	@go run ./tools/skill-lint -repo .

# Validate retired vocabulary and prohibited command guidance across declared
# active instruction surfaces.
lint-instructions:
	@go run ./tools/instruction-lint -repo .

# Flag the single-line bold metadata "header block" anti-pattern (two or more
# **Label:** fields crammed onto one physical line near the top of a doc).
# See docs/doc-header-format.md for the canonical replacement format.
lint-headers:
	@go run ./tools/header-lint -repo .

# Validate every declared, Git-tracked ADR scope, aggregate, and index through
# the repository contract in .dear-agent.yml.
lint-adrs:
	@go run ./tools/adr-lint -repo .

verify-surface-codegen:
	@set -e; \
		before="$$(git hash-object agm/internal/surface/codegen_cli.go agm/internal/surface/codegen_mcp.go agm/internal/surface/codegen_parity_test.go)"; \
		cd agm && go run ./internal/surface/cmd/generate; \
		cd ..; \
		after="$$(git hash-object agm/internal/surface/codegen_cli.go agm/internal/surface/codegen_mcp.go agm/internal/surface/codegen_parity_test.go)"; \
		test "$$before" = "$$after" || { echo "generated AGM surface artifacts are stale" >&2; exit 1; }

plugin-verify-hashes:
	@cd agm && go run ./cmd/plugin-hash -check

# Fast local CI-parity gates. Runs the same go vet / go build / golangci-lint
# CI does, no Docker needed. Catches ~all lint failures in ~25s on a warm
# build cache. See vbonnet/engram-research
# retrospectives/2026-05-27-ci-shift-left.md for the rationale
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

# NOTE: there is intentionally no `install-preflight-hook` target. On this host
# git's effective hooks dir is set globally via core.hooksPath (chezmoi-managed,
# ~/.config/git/hooks), so a repo-local pre-push hook is silently bypassed. The
# global pre-push hook already runs `make preflight` on default-branch pushes
# for any repo that defines a `preflight` target — so a per-repo installer would
# be both redundant and a silent no-op. See bead ce-hft2.

# Install the post-merge worktree-sweep trigger. After a PR lands on the
# default branch locally (e.g. `git pull` on main), it kicks off the canonical
# fail-safe reaper `agm worktree sweep --execute`. The installer honours
# core.hooksPath and refuses to clobber a chezmoi-managed hooks dir — see
# cmd/install-post-merge-hook for the resolution and safety logic.
install-post-merge-hook:
	@go run ./cmd/install-post-merge-hook

# Build the routing-guard binary into ./build (used by the pre-commit hook and
# for local `routing-guard --all` audits).
build-routing-guard:
	@mkdir -p build && go build -o build/routing-guard ./cmd/routing-guard && \
		echo "Built: build/routing-guard"

# Install the routing-guard pre-commit hook. It blocks temporal artifacts
# (Wayfinder runs, retros, designs, research) from being committed to this
# code repo — a thin wrapper over the routing-guard tool (cmd/routing-guard),
# so the forbidden globs come from .dear-agent.yml (no drift). core.hooksPath-
# aware: if hooks are managed by chezmoi (~/.config), it prints how to wire the
# guard into the global dispatcher instead of silently writing a hook that
# won't run. CI enforces the same rule on every PR regardless of local install.
install-routing-guard-hook: build-routing-guard
	@ROOT="$$(git rev-parse --show-toplevel)"; \
	chmod +x "$$ROOT/scripts/git-hooks/pre-commit"; \
	HP="$$(git config --get core.hooksPath || true)"; \
	if [ -n "$$HP" ]; then \
		case "$$HP" in \
		  "$$HOME"/.config/*|"$$HOME"/.local/share/chezmoi/*|~/.config/*|~/.local/share/chezmoi/*|'~/'*) \
			echo "core.hooksPath is chezmoi-managed: $$HP"; \
			echo "A repo-local hook will NOT run on this host. Wire the guard into"; \
			echo "the global pre-commit dispatcher with a line like:"; \
			echo "    ( cd \"$$ROOT\" && exec scripts/git-hooks/pre-commit )"; \
			echo "CI (.github/workflows/routing-enforcement.yml) enforces it regardless."; \
			exit 0;; \
		esac; \
		DEST="$$HP/pre-commit"; \
	else \
		DEST="$$(git rev-parse --git-path hooks/pre-commit)"; \
	fi; \
	if [ -e "$$DEST" ] && ! grep -q 'routing-guard\|git-hooks/pre-commit' "$$DEST" 2>/dev/null; then \
		echo "A pre-commit hook already exists at $$DEST"; \
		echo "Merge in: exec \"$$ROOT/scripts/git-hooks/pre-commit\""; \
		exit 1; \
	fi; \
	cp "$$ROOT/scripts/git-hooks/pre-commit" "$$DEST"; \
	chmod +x "$$DEST"; \
	echo "Installed routing-guard pre-commit hook -> $$DEST"

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
	$(call install-go-bin,bin/configure-claude-settings)

# Build safe-push: a git-push wrapper that resets the credential helper chain
# to gh-only (never osxkeychain, which can hang on a headless GUI prompt) and
# never force-pushes. See internal/safegit and vbonnet/engram-research
# retrospectives/2026-06-08-git-push-credential-hang.md.
build-safe-push:
	@echo "Building safe-push..."
	go build $(GOFLAGS) -o bin/safe-push ./cmd/safe-push/
	@echo "Built: bin/safe-push"

# Install safe-push to GOPATH/bin so it is on PATH for every agent session.
install-safe-push: build-safe-push
	$(call install-go-bin,bin/safe-push)

# Build safe-merge: the vetted, gated PR merger that replaces raw `gh pr merge`.
# Enforces AGENTS.md principle 9: required CI gates, review thread check, soak
# time, and bot review before merge. Raw `gh pr merge` should be denied via a
# PreToolUse hook pointing at this binary (see docs/design/safe-merge.md).
build-safe-merge:
	@echo "Building safe-merge..."
	go build $(GOFLAGS) -o bin/safe-merge ./cmd/safe-merge/
	@echo "Built: bin/safe-merge"

# Install safe-merge to GOPATH/bin so it is on PATH for every agent session.
install-safe-merge: build-safe-merge
	$(call install-go-bin,bin/safe-merge)

# Build safe-rebase: rebase feature branches onto main with safety checks.
# Refuses protected branches, aborts on conflict, optionally force-pushes
# + runs preflight in --auto mode.
build-safe-rebase:
	@echo "Building safe-rebase..."
	go build $(GOFLAGS) -o bin/safe-rebase ./cmd/safe-rebase/
	@echo "Built: bin/safe-rebase"

# Install safe-rebase to GOPATH/bin.
install-safe-rebase: build-safe-rebase
	$(call install-go-bin,bin/safe-rebase)

# Build token-refresher: single-owner, file-locked Claude Code OAuth refresher
# for the VROOM supervisor mesh. Keeps ~/.claude/.credentials.json fresh so
# expired access tokens stop killing the mesh (ce-rnpt / ce-f3e3).
build-token-refresher:
	@echo "Building token-refresher..."
	go build $(GOFLAGS) -o bin/token-refresher ./cmd/token-refresher/
	@echo "Built: bin/token-refresher"

# Install token-refresher to GOPATH/bin.
install-token-refresher: build-token-refresher
	$(call install-go-bin,bin/token-refresher)

# Wire token-refresher into the supervisor mesh (ce-cs3v): deploy the launchd
# idle-backstop that refreshes ~/.claude/.credentials.json every 30 minutes, and
# print the single host-side, ask-gated activation step for you to run yourself.
#
# The scheduled job is the ONLY sanctioned wiring. Do NOT also point Claude
# Code's apiKeyHelper at this binary: since claude-code 2.1.205 a configured
# apiKeyHelper is treated as an external API key that SHADOWS a healthy OAuth
# login and refuses to fall back to it, so the CLI fails with "Invalid API key"
# even when credentials.json is perfectly fresh (anthropics/claude-code#11587,
# #9694, #23568). That wiring used to be step 2 here; it caused a multi-day mesh
# outage and was removed from the host on 2026-07-10. See cmd/token-refresher/
# README.md ("Retired wiring").
install-token-refresher-launchagent: install-token-refresher
	@mkdir -p $(HOME)/Library/LaunchAgents
	@mkdir -p $(HOME)/.local/state/dear-agent
	@sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.dear-agent.token-refresher.plist \
		> $(HOME)/Library/LaunchAgents/com.dear-agent.token-refresher.plist
	@echo "Staged: $(HOME)/Library/LaunchAgents/com.dear-agent.token-refresher.plist"
	@echo "Activate it yourself (ask-gated host action):"
	@echo "  Schedule the idle backstop:"
	@echo "     launchctl load $(HOME)/Library/LaunchAgents/com.dear-agent.token-refresher.plist"
	@if grep -q '"apiKeyHelper"' $(HOME)/.claude/settings.json 2>/dev/null; then \
		echo ""; \
		echo "  WARNING: this host still has a retired apiKeyHelper in ~/.claude/settings.json."; \
		echo "  It shadows healthy OAuth (claude-code >=2.1.205) and will keep breaking auth"; \
		echo "  even with this launch agent running. Clear it:"; \
		echo "     configure-claude-settings remove apiKeyHelper"; \
	fi

# Uninstall still tells you to clear the retired apiKeyHelper. Setup guidance
# for it is gone (see install target), but a host that followed the OLD steps
# has the harmful value sitting in ~/.claude/settings.json, where it keeps
# shadowing healthy OAuth long after this launch agent is gone. Removing the
# setup step without keeping the cleanup step would strand exactly those hosts.
uninstall-token-refresher-launchagent:
	@echo "Disable it yourself, then remove the plist:"
	@echo "  launchctl bootout gui/$$(id -u)/com.dear-agent.token-refresher"
	@echo "If this host ever followed the retired apiKeyHelper instructions, clear it too:"
	@echo "  configure-claude-settings remove apiKeyHelper"
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.token-refresher.plist
	@echo "Removed plist (if present)."

# Build safe-pr: wayfinder-traced wrapper for gh pr create/close.
build-safe-pr:
	@echo "Building safe-pr..."
	go build $(GOFLAGS) -o bin/safe-pr ./cmd/safe-pr/
	@echo "Built: bin/safe-pr"

# Install safe-pr to GOPATH/bin.
install-safe-pr: build-safe-pr
	$(call install-go-bin,bin/safe-pr)

# Build src-recovery: the one sanctioned writer to ~/src/**. It restores a
# golden checkout to a clean, current default branch via exactly stash ->
# checkout default -> pull --ff-only, takes no pass-through git args, and
# refuses every other git verb by construction. See internal/safesrc and
# vbonnet/engram-research retrospectives/2026-06-11-src-violations-and-burndown.md.
build-src-recovery:
	@echo "Building src-recovery..."
	go build $(GOFLAGS) -o bin/src-recovery ./cmd/src-recovery/
	@echo "Built: bin/src-recovery"

# Install src-recovery to GOPATH/bin so it is on PATH for every agent session.
# Allow-list `Bash(src-recovery *)` in chezmoi (dot_claude/private_settings.json.tmpl)
# alongside chezmoi-deploy and safe-push — its safety is guaranteed by
# construction, so it needs no per-invocation approval (AGENTS.md principle 9).
install-src-recovery: build-src-recovery
	$(call install-go-bin,bin/src-recovery)

# Build safe-unlock: the vetted path for clearing stale git lock files from any
# repo or linked worktree. Removes a lock only when it is older than --min-age
# AND held open by no process (lsof), refusing an active one — so it replaces a
# raw `rm .git/index.lock` that would race a live git. Generalises src-recovery's
# ~/src-scoped `unlock` to ~/worktrees/** and the full lock family. See
# internal/safeunlock.
build-safe-unlock:
	@echo "Building safe-unlock..."
	go build $(GOFLAGS) -o bin/safe-unlock ./cmd/safe-unlock/
	@echo "Built: bin/safe-unlock"

# Install safe-unlock to GOPATH/bin so it is on PATH for every agent session.
# Allow-list `Bash(safe-unlock *)` in chezmoi alongside src-recovery/safe-push —
# its safety is guaranteed by construction, so it needs no per-invocation
# approval (AGENTS.md principle 9).
install-safe-unlock: build-safe-unlock
	$(call install-go-bin,bin/safe-unlock)

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
	$(call install-go-bin,bin/jaeger-health)

# Build otel-local: launches a local Jaeger v2 collector (OTLP gRPC :4317,
# UI :16686) with no Docker. Locates or --fetch'es the native Jaeger binary,
# waits for health, and prints the OTEL_EXPORTER_OTLP_ENDPOINT export line.
# See pkg/otelsetup/README.md for the full opt-in tracing workflow.
build-otel-local:
	@echo "Building otel-local..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/otel-local ./cmd/otel-local/
	@echo "Built: bin/otel-local"

install-otel-local: build-otel-local
	$(call install-go-bin,bin/otel-local)

# Convenience: install and launch the local collector in one step. Fetches the
# pinned Jaeger release if no binary is present, then runs it in the foreground
# (Ctrl-C to stop). Then in another shell: eval "$$(otel-local env)".
otel-up: install-otel-local
	$(HOME)/go/bin/otel-local up --fetch

# Build bead-pr-sync: reconciles bead CLOSED status against GitHub PR merge
# state. Finds beads closed while their PR is still open (DoD violations) and
# reopens them to in_progress. See cmd/bead-pr-sync and ce-vqju.
build-bead-pr-sync:
	@echo "Building bead-pr-sync..."
	go build $(GOFLAGS) -o bin/bead-pr-sync ./cmd/bead-pr-sync/
	@echo "Built: bin/bead-pr-sync"

install-bead-pr-sync: build-bead-pr-sync
	$(call install-go-bin,bin/bead-pr-sync)

# Deploy bead-pr-sync as a launchd agent running every 4 hours (ce-yf2c).
# Stages the plist into ~/Library/LaunchAgents and prints the activation
# command (launchctl load is ask-gated; activate it yourself).
install-bead-pr-sync-launchagent: install-bead-pr-sync
	@mkdir -p $(HOME)/Library/LaunchAgents
	@mkdir -p $(HOME)/.local/state/dear-agent
	@sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.dear-agent.bead-pr-sync.plist \
		> $(HOME)/Library/LaunchAgents/com.dear-agent.bead-pr-sync.plist
	@echo "Staged: $(HOME)/Library/LaunchAgents/com.dear-agent.bead-pr-sync.plist"
	@echo "Activate it yourself (ask-gated host action):"
	@echo "  launchctl load $(HOME)/Library/LaunchAgents/com.dear-agent.bead-pr-sync.plist"

uninstall-bead-pr-sync-launchagent:
	@echo "Disable it yourself, then remove the plist:"
	@echo "  launchctl bootout gui/$$(id -u)/com.dear-agent.bead-pr-sync"
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.bead-pr-sync.plist
	@echo "Removed plist (if present)."

# Checks for an existing open PR claiming a bead before creating a new one.
# Usage: bead-pr-guard --bead <id> [--repo owner/name]
build-bead-pr-guard:
	@echo "Building bead-pr-guard..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/bead-pr-guard ./cmd/bead-pr-guard/
	@echo "Built: bin/bead-pr-guard"

install-bead-pr-guard: build-bead-pr-guard
	$(call install-go-bin,bin/bead-pr-guard)

# Supplies only the fixed JSON filters used by attested Codex hook scripts as a
# static Go binary. The privileged install is digest-confirmed and root-staged
# so the unattended agent cannot replace either the executable or its runtime.
build-codex-hook-json:
	@echo "Building codex-hook-json..."
	@mkdir -p bin
	CGO_ENABLED=0 go build $(GOFLAGS) -o bin/codex-hook-json ./cmd/codex-hook-json/
	@echo "Built: bin/codex-hook-json"

install-codex-hook-json: build-codex-hook-json
	@set -eu; \
		test -t 0 || { echo "refusing non-interactive privileged Codex hook JSON helper installation" >&2; exit 2; }; \
		root_group="$$(id -gn 0)"; \
		artifact="bin/codex-hook-json"; \
		helper="/usr/local/libexec/dear-agent-codex-hook-json"; \
		helper_staging=""; \
		expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"; \
		expected_hash="$${expected_hash%% *}"; \
		printf 'Reviewed Codex hook JSON helper SHA-256: %s\n' "$$expected_hash"; \
		printf 'Type that complete SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_hash; \
		test "$$confirmed_hash" = "$$expected_hash" || { echo "Codex hook JSON helper digest confirmation did not match" >&2; exit 2; }; \
		cleanup_helper_staging() { \
			if test -n "$$helper_staging"; then \
				/usr/bin/sudo /bin/rm -f "$$helper_staging" >/dev/null 2>&1 || true; \
			fi; \
		}; \
		trap cleanup_helper_staging EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		if /usr/bin/sudo -n -v 2>/dev/null; then \
			echo "refusing passwordless sudo validation; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		/usr/bin/sudo -v; \
		/usr/bin/sudo /usr/bin/install -d -o root -g "$$root_group" -m 0755 /usr/local/libexec; \
		helper_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-codex-hook-json.XXXXXX)"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0755 "$$artifact" "$$helper_staging"; \
		staged_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$helper_staging")"; \
		staged_hash="$${staged_hash%% *}"; \
		test "$$staged_hash" = "$$expected_hash" || { echo "root-owned staged Codex hook JSON helper differs from the approved bytes" >&2; exit 1; }; \
		/usr/bin/sudo /bin/mv -f "$$helper_staging" "$$helper"; \
		helper_staging=""; \
		trap - EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		echo "Installed digest-bound operator-owned Codex hook JSON helper: $$helper"

# Enforces Definition of Done before bead closure: blocks `bd close` when
# referenced PRs are not yet merged. Used by the pretool-bead-close-guard hook.
# Usage: bead-close-guard --bead <id> [--repo owner/name] [--beads-dir /path]
build-bead-close-guard:
	@echo "Building bead-close-guard..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/bead-close-guard ./cmd/bead-close-guard/
	@echo "Built: bin/bead-close-guard"

install-bead-close-guard: build-bead-close-guard
	@set -eu; \
		test -t 0 || { echo "refusing non-interactive privileged bead-close guard installation" >&2; exit 2; }; \
		root_group="$$(id -gn 0)"; \
		artifact="bin/bead-close-guard"; \
		guard="/usr/local/libexec/dear-agent-bead-close-guard"; \
		guard_staging=""; \
		expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"; \
		expected_hash="$${expected_hash%% *}"; \
		printf 'Reviewed bead-close guard SHA-256: %s\n' "$$expected_hash"; \
		printf 'Type that complete SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_hash; \
		test "$$confirmed_hash" = "$$expected_hash" || { echo "bead-close guard digest confirmation did not match" >&2; exit 2; }; \
		cleanup_guard_staging() { \
			if test -n "$$guard_staging"; then \
				/usr/bin/sudo /bin/rm -f "$$guard_staging" >/dev/null 2>&1 || true; \
			fi; \
		}; \
		trap cleanup_guard_staging EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		if /usr/bin/sudo -n -v 2>/dev/null; then \
			echo "refusing passwordless sudo validation; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		/usr/bin/sudo -v; \
		/usr/bin/sudo /usr/bin/install -d -o root -g "$$root_group" -m 0755 /usr/local/libexec; \
		guard_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-bead-close-guard.XXXXXX)"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0755 "$$artifact" "$$guard_staging"; \
		staged_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$guard_staging")"; \
		staged_hash="$${staged_hash%% *}"; \
		test "$$staged_hash" = "$$expected_hash" || { echo "root-owned staged bead-close guard differs from the approved bytes" >&2; exit 1; }; \
		/usr/bin/sudo /bin/mv -f "$$guard_staging" "$$guard"; \
		guard_staging=""; \
		trap - EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		echo "Installed digest-bound operator-owned Codex hook guard: $$guard"
	$(call install-go-bin,bin/bead-close-guard)

# Detects deployment drift: deployed artifacts (Claude Code hooks, launchd
# plists, chezmoi files) whose source of truth in main no longer matches the
# copy on the host — a fix merged to git but never redeployed (PR #456). Cheap
# hash compare, no builds. See cmd/drift-check/README.md.
build-drift-check:
	@echo "Building drift-check..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/drift-check ./cmd/drift-check/
	@echo "Built: bin/drift-check"

install-drift-check: build-drift-check
	$(call install-go-bin,bin/drift-check)

# Run the legacy hash-only drift check against cmd/drift-check's built-in
# targets. Superseded by `make deploy-status` (manifest-driven, also covers Go
# binaries via version stamp); kept for the agm-hook targets not yet migrated
# into deploy/manifest.yaml. Exit 2 signals drift.
drift-check-legacy: build-drift-check
	./bin/drift-check

# Manifest-driven drift audit — the replacement for `make drift-check`. Builds
# dear-deploy + the write-guard hooks (their source is the compiled binary), then
# compares every artifact in deploy/manifest.yaml to the host: file artifacts by
# content hash, Go binaries by embedded vcs.revision vs repo HEAD. Exit 2 signals
# drift or a missing required artifact, which make surfaces as an error.
deploy-status: build-dear-deploy build-write-guards
	./bin/dear-deploy status

# `make drift-check` now runs the manifest-driven audit (deploy-status). The
# old hash-only detector remains available as `make drift-check-legacy`.
drift-check: deploy-status

# Build babysit-prs: the serial PR updater + merger that works around the
# "every merge makes remaining PRs BEHIND" problem from requiresLinearHistory=true.
# For each open PR: gh pr update-branch --rebase (if BEHIND), then safe-merge.
# Backpressure: exits if open-PR count > --cap (default 50). See ce-5w0i.
build-babysit-prs:
	@echo "Building babysit-prs..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/babysit-prs ./cmd/babysit-prs/
	@echo "Built: bin/babysit-prs"

install-babysit-prs: build-babysit-prs
	$(call install-go-bin,bin/babysit-prs)

# Build mergeloop: the Ralph Wiggum persistent PR-merge loop (ADR-029). Drives
# every open PR toward MERGED with zero human mechanics — rebases behind
# branches, spawns agents to fix CI/conflicts (--enable-agents), and delegates
# the squash-merge to safe-merge. Escalates only for policy blocks. See
# internal/mergeloop and ce-sbnd.
build-mergeloop:
	@echo "Building mergeloop..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/mergeloop ./cmd/mergeloop/
	@echo "Built: bin/mergeloop"

install-mergeloop: build-mergeloop
	$(call install-go-bin,bin/mergeloop)

# Install the launchd agent that runs `mergeloop tick` on an interval. The
# plist is rendered from deploy/launchd/com.dear-agent.mergeloop.plist with the
# real $$HOME substituted. NOTE: this only stages the plist — activating it
# (launchctl bootstrap) is an ask-gated host action you must run yourself; the
# target prints the exact command. This honors Defer-Don't-Block.
install-mergeloop-launchagent: install-mergeloop
	@mkdir -p $(HOME)/Library/LaunchAgents
	@sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.dear-agent.mergeloop.plist \
		> $(HOME)/Library/LaunchAgents/com.dear-agent.mergeloop.plist
	@echo "Staged: $(HOME)/Library/LaunchAgents/com.dear-agent.mergeloop.plist"
	@echo "Activate it yourself (ask-gated host action):"
	@echo "  launchctl bootstrap gui/$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.mergeloop.plist"

uninstall-mergeloop-launchagent:
	@echo "Disable it yourself, then remove the plist:"
	@echo "  launchctl bootout gui/$$(id -u)/com.dear-agent.mergeloop"
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.mergeloop.plist
	@echo "Removed plist (if present)."

# Build resolve-review-threads: atomic wrapper for the resolveReviewThread
# GraphQL mutation. Agents must use this instead of raw `gh api graphql`
# because the classifier blocks bare GraphQL mutations. The binary shells out
# to `gh api graphql` internally, so authentication uses the gh CLI token.
# Usage: resolve-review-threads resolve-all <owner> <repo> <pr> [author]
build-resolve-review-threads:
	@echo "Building resolve-review-threads..."
	go build $(GOFLAGS) -o bin/resolve-review-threads ./cmd/resolve-review-threads/
	@echo "Built: bin/resolve-review-threads"

install-resolve-review-threads: build-resolve-review-threads
	$(call install-go-bin,bin/resolve-review-threads)

# Build merge-audit: safe-merge P6 detection tier. Weekly cross-repo sweep for
# unresolved-threads-at-merge, checks-incomplete-at-merge, direct pushes,
# break-glass overrides, and ruleset drift. Files a P1 bead per violation.
build-merge-audit:
	@echo "Building merge-audit..."
	go build $(GOFLAGS) -o bin/merge-audit ./cmd/merge-audit/
	@echo "Built: bin/merge-audit"

install-merge-audit: build-merge-audit
	$(call install-go-bin,bin/merge-audit)

# Build dear-deploy: the write-side counterpart to drift-check. It deploys host
# artifacts (launchd plists, Claude Code hooks) from deploy/manifest.yaml through
# the principle-9 atomic sequence (stage -> verify -> activate). There is no
# bypass flag (ADR-031); a failed deploy leaves the prior artifact untouched.
build-dear-deploy:
	@echo "Building dear-deploy..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/dear-deploy ./cmd/dear-deploy/
	@echo "Built: bin/dear-deploy"

install-dear-deploy: build-dear-deploy
	$(call install-go-bin,bin/dear-deploy)

# Deploy every artifact in deploy/manifest.yaml to the host. The write-guard
# hooks are compiled first (their source is the built binary under bin/), then
# dear-deploy atomically syncs anything that has drifted. Idempotent.
dear-deploy-sync: build-dear-deploy build-write-guards
	./bin/dear-deploy sync

# Install the kernel FD-ceiling LaunchDaemon (ce-710r.4, R.5 from gopls retro).
# Requires sudo — raises kern.maxfiles from 184320 to 524288 at boot so a
# gopls FD storm degrades performance rather than hard-deadlocking go build.
install-fd-limit-launchdaemon:
	@echo "Installing com.dear-agent.fd-limit LaunchDaemon (requires root)..."
	sudo cp deploy/launchd/com.dear-agent.fd-limit.plist \
		/Library/LaunchDaemons/com.dear-agent.fd-limit.plist
	sudo chmod 644 /Library/LaunchDaemons/com.dear-agent.fd-limit.plist
	sudo chown root:wheel /Library/LaunchDaemons/com.dear-agent.fd-limit.plist
	sudo launchctl load -w /Library/LaunchDaemons/com.dear-agent.fd-limit.plist
	@echo "✓ fd-limit LaunchDaemon loaded (kern.maxfiles=524288 will apply at boot)"
	@echo "  Apply immediately without reboot: sudo sysctl -w kern.maxfiles=524288 kern.maxfilesperproc=262144"

uninstall-fd-limit-launchdaemon:
	@echo "Uninstalling com.dear-agent.fd-limit LaunchDaemon..."
	-sudo launchctl unload /Library/LaunchDaemons/com.dear-agent.fd-limit.plist 2>/dev/null
	sudo rm -f /Library/LaunchDaemons/com.dear-agent.fd-limit.plist
	@echo "✓ fd-limit LaunchDaemon removed"

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
	$(call install-go-bin,bin/pretool-fs-write-guard,$(HOOKS_DIR))
	$(call install-go-bin,bin/pretool-bash-write-guard,$(HOOKS_DIR))

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
	$(call install-go-bin,bin/dear-agent-bumblebee,$(HOME)/.local/bin)
	@$(HOME)/.local/bin/dear-agent-bumblebee install-launchagent

uninstall-bumblebee-launchagent: build-bumblebee
	@./bin/dear-agent-bumblebee install-launchagent --uninstall

build-pr-linkify:
	@echo "Building pr-linkify..."
	go build $(GOFLAGS) -o bin/pr-linkify ./cmd/pr-linkify/
	@echo "Built: bin/pr-linkify"

install-pr-linkify: build-pr-linkify
	$(call install-go-bin,bin/pr-linkify)

# Build fd-pressure: a standalone FD/vnode/gopls pressure monitor. Samples
# system resource state and exits non-zero if any threshold is breached.
# Calls the same SysResourceProbe that the VROOM Overseer uses, so output
# is Overseer-consistent. Useful for human triage and launchd health checks.
build-fd-pressure:
	@echo "Building fd-pressure..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/fd-pressure ./cmd/fd-pressure/
	@echo "Built: bin/fd-pressure"

install-fd-pressure: build-fd-pressure
	$(call install-go-bin,bin/fd-pressure)

build-gopls-watchdog:
	@echo "Building gopls-watchdog..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/gopls-watchdog ./cmd/gopls-watchdog/
	@echo "Built: bin/gopls-watchdog"

install-gopls-watchdog: build-gopls-watchdog
	$(call install-go-bin,bin/gopls-watchdog)

install-gopls-watchdog-launchagent: install-gopls-watchdog
	@mkdir -p $(HOME)/Library/LaunchAgents
	@mkdir -p $(HOME)/.local/state/dear-agent
	@sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.dear-agent.gopls-watchdog.plist \
		> $(HOME)/Library/LaunchAgents/com.dear-agent.gopls-watchdog.plist
	@echo "Staged: $(HOME)/Library/LaunchAgents/com.dear-agent.gopls-watchdog.plist"
	@echo "Activate it yourself (ask-gated host action):"
	@echo "  launchctl bootstrap gui/$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.gopls-watchdog.plist"

uninstall-gopls-watchdog-launchagent:
	@launchctl bootout gui/$$(id -u)/com.dear-agent.gopls-watchdog 2>/dev/null || true
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.gopls-watchdog.plist
	@echo "Removed: com.dear-agent.gopls-watchdog launch agent"

# Stage the hourly sandbox-dir GC launch agent (ce-uxju). Staging only —
# activation is a separate ask-gated `launchctl bootstrap`, and the plist
# header says to run a manual `agm sandbox gc` dry run first.
install-sandbox-gc-launchagent: install-agm
	@mkdir -p $(HOME)/Library/LaunchAgents
	@mkdir -p $(HOME)/.local/state/dear-agent
	@sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.dear-agent.sandbox-gc.plist \
		> $(HOME)/Library/LaunchAgents/com.dear-agent.sandbox-gc.plist
	@echo "Staged: $(HOME)/Library/LaunchAgents/com.dear-agent.sandbox-gc.plist"
	@echo "Review a dry run first: agm sandbox gc"
	@echo "Activate it yourself (ask-gated host action):"
	@echo "  launchctl bootstrap gui/$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.sandbox-gc.plist"

uninstall-sandbox-gc-launchagent:
	@launchctl bootout gui/$$(id -u)/com.dear-agent.sandbox-gc 2>/dev/null || true
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.sandbox-gc.plist
	@echo "Removed: com.dear-agent.sandbox-gc launch agent"
build-disk-watchdog:
	@echo "Building disk-watchdog..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/disk-watchdog ./cmd/disk-watchdog/
	@echo "Built: bin/disk-watchdog"

install-disk-watchdog: build-disk-watchdog
	$(call install-go-bin,bin/disk-watchdog)

install-disk-watchdog-launchagent: install-disk-watchdog
	@mkdir -p $(HOME)/Library/LaunchAgents
	@mkdir -p $(HOME)/.local/state/dear-agent
	@sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.dear-agent.disk-watchdog.plist \
		> $(HOME)/Library/LaunchAgents/com.dear-agent.disk-watchdog.plist
	@echo "Staged: $(HOME)/Library/LaunchAgents/com.dear-agent.disk-watchdog.plist"
	@echo "Activate it yourself (ask-gated host action):"
	@echo "  launchctl bootstrap gui/$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.disk-watchdog.plist"

uninstall-disk-watchdog-launchagent:
	@launchctl bootout gui/$$(id -u)/com.dear-agent.disk-watchdog 2>/dev/null || true
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.disk-watchdog.plist
	@echo "Removed: com.dear-agent.disk-watchdog launch agent"

# On Unix systems without macOS authopen, authorized uses append through this
# one-purpose root-owned helper. Installation is an explicit operator action:
# it requires a fresh interactive sudo challenge and installs a NOPASSWD rule
# only for the helper's exact path, never for AGM, tee, chmod, or a variable
# destination.
build-override-ledger-helper:
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/dear-agent-override-ledger-append ./cmd/override-ledger-append/

install-override-ledger-helper: build-override-ledger-helper install-agm
	@set -eu; \
		test -t 0 || { echo "refusing non-interactive privileged helper installation" >&2; exit 2; }; \
		operator_user="$$(id -un)"; \
		root_group="$$(id -gn 0)"; \
		artifact="bin/dear-agent-override-ledger-append"; \
		agm_executable="$(HOME)/go/bin/agm"; \
		helper="/usr/local/libexec/dear-agent-override-ledger-append"; \
		helper_staging=""; \
		identity="/usr/local/libexec/dear-agent-override-ledger-agm.identity"; \
		identity_staging=""; \
		sudoers="/etc/sudoers.d/dear-agent-override-ledger"; \
		staging="/etc/sudoers.d/.dear-agent-override-ledger.$$$$"; \
		expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"; \
		expected_hash="$${expected_hash%% *}"; \
		case "$$(uname -s)" in \
			Darwin) \
				caller_digest="$$(/usr/bin/codesign -dvvv "$$agm_executable" 2>&1 | /usr/bin/sed -n 's/^CDHash=//p' | /usr/bin/tr '[:upper:]' '[:lower:]')"; \
				test -n "$$caller_digest" || { echo "installed AGM has no kernel-verifiable code identity" >&2; exit 1; }; \
				caller_identity="darwin-cdhash:$$caller_digest"; \
				;; \
			Linux) \
				caller_digest="$$(/usr/bin/sha256sum "$$agm_executable")"; \
				caller_digest="$${caller_digest%% *}"; \
				caller_identity="linux-sha256:$$caller_digest"; \
				;; \
			*) echo "authenticated ledger callers are unsupported on this platform" >&2; exit 2 ;; \
		esac; \
		rule="$$operator_user ALL=(root) NOPASSWD: sha256:$$expected_hash $$helper"; \
		printf 'Reviewed helper SHA-256: %s\n' "$$expected_hash"; \
		printf 'Type that complete SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_hash; \
		test "$$confirmed_hash" = "$$expected_hash" || { echo "helper digest confirmation did not match" >&2; exit 2; }; \
		printf 'Reviewed installed AGM caller identity: %s\n' "$$caller_identity"; \
		printf 'Type that complete identity to bind privileged appends to these exact AGM bytes: '; \
		IFS= read -r confirmed_identity; \
		test "$$confirmed_identity" = "$$caller_identity" || { echo "AGM caller identity confirmation did not match" >&2; exit 2; }; \
		cleanup_helper_staging() { \
			if test -n "$$helper_staging"; then \
				/usr/bin/sudo -n /bin/rm -f "$$helper_staging" >/dev/null 2>&1 || true; \
			fi; \
			if test -n "$$identity_staging"; then \
				/usr/bin/sudo -n /bin/rm -f "$$identity_staging" >/dev/null 2>&1 || true; \
			fi; \
			if test -n "$$staging"; then \
				/usr/bin/sudo -n /bin/rm -f "$$staging" >/dev/null 2>&1 || true; \
			fi; \
			/usr/bin/sudo -k >/dev/null 2>&1 || true; \
		}; \
		trap cleanup_helper_staging EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		if /usr/bin/sudo -n -v 2>/dev/null; then \
			echo "refusing passwordless sudo validation; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		/usr/bin/sudo -v; \
		/usr/bin/sudo /usr/bin/install -d -o root -g "$$root_group" -m 0755 /usr/local/libexec; \
		identity_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-ledger-agm.identity.XXXXXX)"; \
		printf '%s\n' "$$caller_identity" | /usr/bin/sudo /usr/bin/tee "$$identity_staging" >/dev/null; \
		/usr/bin/sudo /bin/chmod 0444 "$$identity_staging"; \
		staged_identity="$$(/usr/bin/sudo /bin/cat "$$identity_staging")"; \
		test "$$staged_identity" = "$$caller_identity" || { echo "root-owned staged AGM caller identity differs from the approved identity" >&2; exit 1; }; \
		helper_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-ledger-append.XXXXXX)"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0755 "$$artifact" "$$helper_staging"; \
		staged_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$helper_staging")"; \
		staged_hash="$${staged_hash%% *}"; \
		test "$$staged_hash" = "$$expected_hash" || { echo "root-owned staged helper differs from the approved bytes" >&2; exit 1; }; \
		printf '%s\n' "$$rule" | /usr/bin/sudo /usr/bin/tee "$$staging" >/dev/null; \
		/usr/bin/sudo /bin/chmod 0440 "$$staging"; \
		if ! /usr/bin/sudo /usr/sbin/visudo -cf "$$staging"; then \
			exit 1; \
		fi; \
		/usr/bin/sudo /bin/mv -f "$$staging" "$$sudoers"; \
		staging=""; \
		/usr/bin/sudo /bin/mv -f "$$identity_staging" "$$identity"; \
		identity_staging=""; \
		/usr/bin/sudo /bin/mv -f "$$helper_staging" "$$helper"; \
		helper_staging=""; \
		trap - EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		echo "Installed digest-bound root-owned ledger helper, AGM caller identity, and exact sudoers rule for $$operator_user"

# Install the macOS audit under launchd's system domain without activating it.
# Both scheduler and executable are root-owned, so an unattended same-user
# agent cannot replace them or disable the job through its GUI launchd domain.
# Installation is an explicit, freshly authenticated operator action.
install-override-audit-launchdaemon: build-agm
	@set -eu; \
		test "$$(uname -s)" = "Darwin" || { echo "launchd audit installation is macOS-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit installation" >&2; exit 2; }; \
		operator_user="$$(id -un)"; \
		case "$$operator_user" in *[!A-Za-z0-9._-]*|"") echo "unsupported operator account name" >&2; exit 2;; esac; \
		root_group="$$(id -gn 0)"; \
		audit_artifact="bin/agm"; \
		plist_candidate="$$(/usr/bin/mktemp "$${TMPDIR:-/tmp}/dear-agent-override-audit.XXXXXX")"; \
		audit_staging=""; \
		plist_staging=""; \
		/usr/bin/sed "s|__OPERATOR_USER__|$$operator_user|g" deploy/launchd/com.dear-agent.override-audit.plist >"$$plist_candidate"; \
		/usr/bin/plutil -lint "$$plist_candidate" >/dev/null; \
		expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"; \
		expected_audit_hash="$${expected_audit_hash%% *}"; \
		expected_plist_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$plist_candidate")"; \
		expected_plist_hash="$${expected_plist_hash%% *}"; \
		printf 'Reviewed audit executable SHA-256: %s\n' "$$expected_audit_hash"; \
		printf 'Reviewed rendered LaunchDaemon SHA-256: %s\n' "$$expected_plist_hash"; \
		printf 'Type the executable SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_audit_hash; \
		printf 'Type the LaunchDaemon SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_plist_hash; \
		test "$$confirmed_audit_hash" = "$$expected_audit_hash" || { echo "audit executable digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_plist_hash" = "$$expected_plist_hash" || { echo "LaunchDaemon digest confirmation did not match" >&2; exit 2; }; \
		cleanup_audit_staging() { \
			/bin/rm -f "$$plist_candidate"; \
			if test -n "$$audit_staging"; then /usr/bin/sudo /bin/rm -f "$$audit_staging" >/dev/null 2>&1 || true; fi; \
			if test -n "$$plist_staging"; then /usr/bin/sudo /bin/rm -f "$$plist_staging" >/dev/null 2>&1 || true; fi; \
		}; \
		trap cleanup_audit_staging EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		if /usr/bin/sudo -n -v 2>/dev/null; then \
			echo "refusing passwordless sudo validation; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		/usr/bin/sudo -v; \
		/usr/bin/sudo /usr/bin/install -d -o root -g "$$root_group" -m 0755 /usr/local/libexec; \
		audit_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit.XXXXXX)"; \
		plist_staging="$$(/usr/bin/sudo /usr/bin/mktemp /Library/LaunchDaemons/.com.dear-agent.override-audit.XXXXXX)"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0755 "$$audit_artifact" "$$audit_staging"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0644 "$$plist_candidate" "$$plist_staging"; \
		staged_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_staging")"; \
		staged_audit_hash="$${staged_audit_hash%% *}"; \
		staged_plist_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$plist_staging")"; \
		staged_plist_hash="$${staged_plist_hash%% *}"; \
		test "$$staged_audit_hash" = "$$expected_audit_hash" || { echo "root-owned staged audit executable differs from the approved bytes" >&2; exit 1; }; \
		test "$$staged_plist_hash" = "$$expected_plist_hash" || { echo "root-owned staged LaunchDaemon differs from the approved bytes" >&2; exit 1; }; \
		/usr/bin/sudo /usr/bin/plutil -lint "$$plist_staging" >/dev/null; \
		/usr/bin/sudo /bin/mv -f "$$audit_staging" /usr/local/libexec/dear-agent-override-audit; \
		audit_staging=""; \
		/usr/bin/sudo /bin/mv -f "$$plist_staging" /Library/LaunchDaemons/com.dear-agent.override-audit.plist; \
		plist_staging=""; \
		/bin/rm -f "$$plist_candidate"; \
		plist_candidate=""; \
		trap - EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		echo "Installed digest-bound root-owned audit executable and system LaunchDaemon"; \
		echo "Review, activate, and monitor it yourself (ask-gated host actions):"; \
		echo "  sudo launchctl bootstrap system /Library/LaunchDaemons/com.dear-agent.override-audit.plist"; \
		echo "  log stream --predicate 'senderImagePath == \"/usr/bin/logger\"'"

uninstall-override-audit-launchdaemon:
	@set -eu; \
		test "$$(uname -s)" = "Darwin" || { echo "launchd audit removal is macOS-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit removal" >&2; exit 2; }; \
		/usr/bin/sudo -k; \
		if /usr/bin/sudo -n -v 2>/dev/null; then \
			echo "refusing passwordless sudo validation; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		/usr/bin/sudo -v; \
		/usr/bin/sudo /bin/launchctl bootout system/com.dear-agent.override-audit 2>/dev/null || true; \
		/usr/bin/sudo /bin/rm -f /Library/LaunchDaemons/com.dear-agent.override-audit.plist /usr/local/libexec/dear-agent-override-audit; \
		/usr/bin/sudo -k; \
		echo "Removed the root-owned dangerous-override audit LaunchDaemon and executable"

# Install the Linux audit under the system manager without activating it. The
# template runs a root-owned AGM copy as the named unprivileged operator, so an
# unattended same-user agent cannot replace the executable or disable the timer
# through `systemctl --user`. Installation is an explicit, freshly
# authenticated operator action.
install-override-audit-systemd: build-agm
	@set -eu; \
		test "$$(uname -s)" = "Linux" || { echo "systemd audit installation is Linux-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit installation" >&2; exit 2; }; \
		command -v systemctl >/dev/null || { echo "systemctl is required" >&2; exit 2; }; \
		operator_user="$$(id -un)"; \
		case "$$operator_user" in *[!A-Za-z0-9._-]*|"") echo "unsupported operator account name" >&2; exit 2;; esac; \
		root_group="$$(id -gn 0)"; \
		audit_artifact="bin/agm"; \
		service_artifact="agm/systemd/dear-agent-override-audit@.service"; \
		timer_artifact="agm/systemd/dear-agent-override-audit@.timer"; \
		audit_staging=""; \
		service_staging=""; \
		timer_staging=""; \
		expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"; \
		expected_audit_hash="$${expected_audit_hash%% *}"; \
		expected_service_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$service_artifact")"; \
		expected_service_hash="$${expected_service_hash%% *}"; \
		expected_timer_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$timer_artifact")"; \
		expected_timer_hash="$${expected_timer_hash%% *}"; \
		printf 'Reviewed audit executable SHA-256: %s\n' "$$expected_audit_hash"; \
		printf 'Reviewed systemd service SHA-256: %s\n' "$$expected_service_hash"; \
		printf 'Reviewed systemd timer SHA-256: %s\n' "$$expected_timer_hash"; \
		printf 'Type the executable SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_audit_hash; \
		printf 'Type the service SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_service_hash; \
		printf 'Type the timer SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_timer_hash; \
		test "$$confirmed_audit_hash" = "$$expected_audit_hash" || { echo "audit executable digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_service_hash" = "$$expected_service_hash" || { echo "systemd service digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_timer_hash" = "$$expected_timer_hash" || { echo "systemd timer digest confirmation did not match" >&2; exit 2; }; \
		cleanup_systemd_staging() { \
			if test -n "$$audit_staging"; then /usr/bin/sudo /bin/rm -f "$$audit_staging" >/dev/null 2>&1 || true; fi; \
			if test -n "$$service_staging"; then /usr/bin/sudo /bin/rm -f "$$service_staging" >/dev/null 2>&1 || true; fi; \
			if test -n "$$timer_staging"; then /usr/bin/sudo /bin/rm -f "$$timer_staging" >/dev/null 2>&1 || true; fi; \
		}; \
		trap cleanup_systemd_staging EXIT HUP INT TERM; \
		/usr/bin/sudo -k; \
		if /usr/bin/sudo -n -v 2>/dev/null; then \
			echo "refusing passwordless sudo validation; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		/usr/bin/sudo -v; \
		/usr/bin/sudo /usr/bin/install -d -o root -g "$$root_group" -m 0755 /usr/local/libexec; \
		audit_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit.XXXXXX)"; \
		service_staging="$$(/usr/bin/sudo /usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-service.XXXXXX)"; \
		timer_staging="$$(/usr/bin/sudo /usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-timer.XXXXXX)"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0755 "$$audit_artifact" "$$audit_staging"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0644 "$$service_artifact" "$$service_staging"; \
		/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0644 "$$timer_artifact" "$$timer_staging"; \
		staged_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_staging")"; \
		staged_audit_hash="$${staged_audit_hash%% *}"; \
		staged_service_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$service_staging")"; \
		staged_service_hash="$${staged_service_hash%% *}"; \
		staged_timer_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$timer_staging")"; \
		staged_timer_hash="$${staged_timer_hash%% *}"; \
		test "$$staged_audit_hash" = "$$expected_audit_hash" || { echo "root-owned staged audit executable differs from the approved bytes" >&2; exit 1; }; \
		test "$$staged_service_hash" = "$$expected_service_hash" || { echo "root-owned staged systemd service differs from the approved bytes" >&2; exit 1; }; \
		test "$$staged_timer_hash" = "$$expected_timer_hash" || { echo "root-owned staged systemd timer differs from the approved bytes" >&2; exit 1; }; \
		/usr/bin/sudo /bin/mv -f "$$audit_staging" /usr/local/libexec/dear-agent-override-audit; \
		audit_staging=""; \
		/usr/bin/sudo /bin/mv -f "$$service_staging" /etc/systemd/system/dear-agent-override-audit@.service; \
		service_staging=""; \
		/usr/bin/sudo /bin/mv -f "$$timer_staging" /etc/systemd/system/dear-agent-override-audit@.timer; \
		timer_staging=""; \
		trap - EXIT HUP INT TERM; \
		/usr/bin/sudo /usr/bin/systemctl daemon-reload; \
		/usr/bin/sudo -k; \
		echo "Installed digest-bound root-owned audit executable and system unit templates"; \
		echo "Review, activate, and monitor them yourself (ask-gated host actions):"; \
		echo "  sudo systemctl enable --now dear-agent-override-audit@$$operator_user.timer"; \
		echo "  journalctl -t dear-agent-override-audit"

uninstall-override-audit-systemd:
	@set -eu; \
		test "$$(uname -s)" = "Linux" || { echo "systemd audit removal is Linux-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit removal" >&2; exit 2; }; \
		operator_user="$$(id -un)"; \
		/usr/bin/sudo -k; \
		if /usr/bin/sudo -n -v 2>/dev/null; then \
			echo "refusing passwordless sudo validation; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		/usr/bin/sudo -v; \
		/usr/bin/sudo /usr/bin/systemctl disable --now "dear-agent-override-audit@$$operator_user.timer" 2>/dev/null || true; \
		/usr/bin/sudo /bin/rm -f /etc/systemd/system/dear-agent-override-audit@.service /etc/systemd/system/dear-agent-override-audit@.timer /usr/local/libexec/dear-agent-override-audit; \
		/usr/bin/sudo /usr/bin/systemctl daemon-reload; \
		/usr/bin/sudo -k; \
		echo "Removed the root-owned dangerous-override audit units and executable"

# gobin-guard (ce-24f1): SENSE + ESCALATE guard for ~/go/bin. Installed OUTSIDE
# ~/go/bin (into ~/.local/state/dear-agent/bin) so it survives the very wipe it
# is meant to detect — a compiled guard living in ~/go/bin would be deleted with
# everything else.
install-gobin-guard:
	@$(MAKE) build-dear-deploy
	./bin/dear-deploy sync gobin-guard gobin-guard-audit
	@echo "Installed: GOBIN guard and independent liveness auditor"

install-gobin-guard-launchagent: install-gobin-guard
	./bin/dear-deploy sync com.dear-agent.gobin-guard com.dear-agent.gobin-guard-audit
	@echo "Staged: GOBIN guard and liveness-audit launch agents"
	@echo "Activate them yourself (ask-gated host action):"
	@echo "  launchctl bootstrap gui/$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.gobin-guard.plist"
	@echo "  launchctl bootstrap gui/$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.gobin-guard-audit.plist"

uninstall-gobin-guard-launchagent:
	@launchctl bootout gui/$$(id -u)/com.dear-agent.gobin-guard 2>/dev/null || true
	@launchctl bootout gui/$$(id -u)/com.dear-agent.gobin-guard-audit 2>/dev/null || true
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.gobin-guard.plist
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.gobin-guard-audit.plist
	@echo "Removed: GOBIN guard launch agents"

build-vroom-dispatch:
	@echo "Building vroom-dispatch..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/vroom-dispatch ./cmd/vroom-dispatch/
	@echo "Built: bin/vroom-dispatch"

install-vroom-dispatch: build-vroom-dispatch
	$(call install-go-bin,bin/vroom-dispatch)

# Build vroom-mesh: in-process 3-supervisor VROOM mesh harness (ce-plf0).
# Supports both in-memory substrates (default) and real adapters:
#   vroom-mesh --beads-db ~/beads/context-engine/.beads  (real roadmap)
#   vroom-mesh --beads-db ... --agm-dispatch             (real roadmap + dispatch)
build-vroom-mesh:
	@echo "Building vroom-mesh..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/vroom-mesh ./cmd/vroom-mesh/
	@echo "Built: bin/vroom-mesh"

install-vroom-mesh: build-vroom-mesh
	$(call install-go-bin,bin/vroom-mesh)

# Build agm-bus channel MCP adapter (permission-relay channel, ce-plf0).
# TypeScript: runs npm install + tsc. Output lands in agm/agm-plugin/channels/agm-bus/dist/.
# Usage after build: node agm/agm-plugin/channels/agm-bus/dist/index.js
# See agm/agm-plugin/channels/agm-bus/README.md for session wiring.
build-agm-bus:
	@echo "Building agm-bus channel (npm)..."
	cd agm/agm-plugin/channels/agm-bus && npm install --prefer-offline --silent && npm run build
	@echo "Built: agm/agm-plugin/channels/agm-bus/dist/index.js"

build-vroom-prompt-gen:
	@echo "Building vroom-prompt-gen..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/vroom-prompt-gen ./cmd/vroom-prompt-gen/
	@echo "Built: bin/vroom-prompt-gen"

install-vroom-prompt-gen: build-vroom-prompt-gen
	$(call install-go-bin,bin/vroom-prompt-gen)

# Build agm-job: the host-side job runner for the dear-agent dispatch loop
# (ce-m3ya, Phase A of ce-cd14). Wraps commands with atomic flock locking,
# mandatory --verify, macOS notification + agm send escalation on failure,
# and self-rotating logs under ~/.agm/logs/.
build-agm-job:
	@echo "Building agm-job..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/agm-job ./cmd/agm-job/
	@echo "Built: bin/agm-job"

install-agm-job: build-agm-job
	$(call install-go-bin,bin/agm-job)

# Build src-health: canary that checks 7 ~/src repos for clean working tree,
# branch, and ahead/behind status. Used to soak the host dispatch loop during
# Phase A of ce-cd14.
build-src-health:
	@echo "Building src-health..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/src-health ./cmd/src-health/
	@echo "Built: bin/src-health"

install-src-health: build-src-health
	$(call install-go-bin,bin/src-health)

# Build burndown-maint: host-side bead-burndown maintenance tick (ce-cd14.2).
# Counts active burndown workers via agm session list, spawns at most 1 per
# tick up to --target (default 1). Run under agm-job for locking/escalation.
build-burndown-maint:
	@echo "Building burndown-maint..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/burndown-maint ./cmd/burndown-maint/
	@echo "Built: bin/burndown-maint"

install-burndown-maint: build-burndown-maint
	$(call install-go-bin,bin/burndown-maint)

# Build vroom-governor: system load + RAM monitor that pauses/resumes spawns
# and archives the newest worker on critical memory pressure (ce-lxdo).
# Interval and thresholds are configurable via flags.
build-vroom-governor:
	@echo "Building vroom-governor..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/vroom-governor ./cmd/vroom-governor/
	@echo "Built: bin/vroom-governor"

install-vroom-governor: build-vroom-governor
	$(call install-go-bin,bin/vroom-governor)

# Build the AGM CLI and its detached archive companion with one version stamp.
# A standalone `go install ./agm/cmd/agm` omits both the stamp and companion;
# prefer `make install-agm` so async archive compatibility remains coherent.
build-agm:
	@echo "Building agm + agm-reaper..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/agm ./agm/cmd/agm/
	go build $(GOFLAGS) -o bin/agm-reaper ./agm/cmd/agm-reaper/
	@echo "Built: bin/agm bin/agm-reaper"

install-agm: build-agm
	$(call install-go-bin,bin/agm)
	$(call install-go-bin,bin/agm-reaper)

build-agm-mcp-server:
	@echo "Building agm-mcp-server..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/agm-mcp-server ./agm/cmd/agm-mcp-server/
	@echo "Built: bin/agm-mcp-server"

install-agm-mcp-server: build-agm-mcp-server
	$(call install-go-bin,bin/agm-mcp-server)

# Build engram-mcp: Go Engram MCP server with verified beads writes (ce-ctsi).
# Supersedes the legacy Python server whose beads_create silently wrote to a
# JSONL file nothing reads. Requires BEADS_DB at runtime for beads tools.
build-engram-mcp:
	@echo "Building engram-mcp..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/engram-mcp ./engram/cmd/engram-mcp/
	@echo "Built: bin/engram-mcp"

install-engram-mcp: build-engram-mcp
	$(call install-go-bin,bin/engram-mcp)

# Build session-skill-extractor: analyzes a completed session transcript and
# proposes a new SKILL candidate via the model (ce-ouvr). Includes dedup
# against existing skills and an anti-pattern guard. Output is a reviewable
# candidate file — never auto-committed to ~/.claude/skills/.
build-session-skill-extractor:
	@echo "Building session-skill-extractor..."
	@mkdir -p bin
	go build $(GOFLAGS) -o bin/session-skill-extractor ./cmd/session-skill-extractor/
	@echo "Built: bin/session-skill-extractor"

install-session-skill-extractor: build-session-skill-extractor
	$(call install-go-bin,bin/session-skill-extractor)
