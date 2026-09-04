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

# Version stamping (ce-wy1q). Injected into every root-Makefile binary build via
# -ldflags so that version-aware binaries report the actual build provenance.
# Override on the CLI: make build-safe-pr VERSION=1.2.3
VERSION    ?= dev
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# GOFLAGS is caller-owned. Governed builds leave ordinary flags to the Go
# command's environment inheritance and reject only competing linker flags.
# Optional unpatterned linker customization has one explicit ingress; protected
# provenance assignments are appended last so callers cannot replace them.
EXTRA_GO_LDFLAGS ?=

override _BUILD_STAMP_PACKAGE := github.com/vbonnet/dear-agent/pkg/version
override _BUILD_STAMP_EXTRA_LDFLAGS := $(value EXTRA_GO_LDFLAGS)
override _BUILD_STAMP_VERSION := $(if $(filter file,$(origin VERSION)),$(VERSION),$(value VERSION))
override _BUILD_STAMP_GIT_COMMIT = $(if $(strip $(value GIT_COMMIT)),$(value GIT_COMMIT),$(or $(shell GOFLAGS=-buildvcs=false GOENV=off GOWORK=off GOOS= GOARCH= go run ./internal/buildstamp git-commit 2>/dev/null),unknown))
override _BUILD_STAMP_DATE := $(if $(filter file,$(origin BUILD_DATE)),$(BUILD_DATE),$(value BUILD_DATE))
override _BUILD_STAMP_TEST_OUTPUT := $(value BUILD_STAMP_TEST_OUTPUT)
override _BUILD_STAMP_RAW_GOFLAGS := $(value GOFLAGS)
override _BUILD_STAMP_RAW_GOENV := $(value GOENV)
unexport EXTRA_GO_LDFLAGS VERSION GIT_COMMIT BUILD_DATE BUILD_STAMP_TEST_OUTPUT
override GOFLAGS := $(_BUILD_STAMP_RAW_GOFLAGS)
export GOFLAGS _BUILD_STAMP_RAW_GOFLAGS _BUILD_STAMP_RAW_GOENV _BUILD_STAMP_EXTRA_LDFLAGS _BUILD_STAMP_VERSION _BUILD_STAMP_GIT_COMMIT _BUILD_STAMP_DATE _BUILD_STAMP_TEST_OUTPUT

# Protected stamp metadata must remain one Go linker token. Name each parser
# delimiter explicitly so the Make-side rejection below cannot be overridden.
override define _BUILD_STAMP_NEWLINE


endef
override _BUILD_STAMP_EMPTY :=
override _BUILD_STAMP_SPACE := $(_BUILD_STAMP_EMPTY) $(_BUILD_STAMP_EMPTY)
override _BUILD_STAMP_TAB := $(shell printf '\t')
override _BUILD_STAMP_CR := $(shell printf '\r')
override _BUILD_STAMP_SQUOTE := '
override _BUILD_STAMP_DQUOTE := "
override _NORMALIZED_EXTRA_GO_LDFLAGS = $(strip $(_BUILD_STAMP_EXTRA_LDFLAGS))
override _INVALID_EXTRA_GO_LDFLAGS = $(if $(_NORMALIZED_EXTRA_GO_LDFLAGS),$(if $(filter -%,$(firstword $(_NORMALIZED_EXTRA_GO_LDFLAGS))),,yes),)
override _NORMALIZED_STAMP_VERSION = $(subst $(_BUILD_STAMP_DQUOTE), ,$(subst $(_BUILD_STAMP_SQUOTE), ,$(subst $(_BUILD_STAMP_CR), ,$(subst $(_BUILD_STAMP_NEWLINE), ,$(subst $(_BUILD_STAMP_TAB), ,$(_BUILD_STAMP_VERSION))))))
override _NORMALIZED_STAMP_GIT_COMMIT = $(subst $(_BUILD_STAMP_DQUOTE), ,$(subst $(_BUILD_STAMP_SQUOTE), ,$(subst $(_BUILD_STAMP_CR), ,$(subst $(_BUILD_STAMP_NEWLINE), ,$(subst $(_BUILD_STAMP_TAB), ,$(_BUILD_STAMP_GIT_COMMIT))))))
override _NORMALIZED_STAMP_DATE = $(subst $(_BUILD_STAMP_DQUOTE), ,$(subst $(_BUILD_STAMP_SQUOTE), ,$(subst $(_BUILD_STAMP_CR), ,$(subst $(_BUILD_STAMP_NEWLINE), ,$(subst $(_BUILD_STAMP_TAB), ,$(_BUILD_STAMP_DATE))))))
override _UNSAFE_STAMP_FIELDS = $(if $(findstring $(_BUILD_STAMP_SPACE),$(_NORMALIZED_STAMP_VERSION)),VERSION) $(if $(findstring $(_BUILD_STAMP_SPACE),$(_NORMALIZED_STAMP_GIT_COMMIT)),GIT_COMMIT) $(if $(findstring $(_BUILD_STAMP_SPACE),$(_NORMALIZED_STAMP_DATE)),BUILD_DATE)
override _MANDATORY_VERSION_LDFLAGS = \
	$(if $(strip $(_UNSAFE_STAMP_FIELDS)),$(error build stamp metadata $(strip $(_UNSAFE_STAMP_FIELDS)) must not contain space tab newline carriage return or quote),) \
	-X $(_BUILD_STAMP_PACKAGE).Version=$${_BUILD_STAMP_VERSION} \
	-X $(_BUILD_STAMP_PACKAGE).GitCommit=$${_BUILD_STAMP_GIT_COMMIT} \
	-X $(_BUILD_STAMP_PACKAGE).BuildDate=$${_BUILD_STAMP_DATE} \
	-X $(_BUILD_STAMP_PACKAGE).BuiltBy=makefile
override _BUILD_STAMP_LDFLAGS = $${_BUILD_STAMP_EXTRA_LDFLAGS} $(_MANDATORY_VERSION_LDFLAGS)
override BUILD_STAMP_FLAGS = $(if $(_INVALID_EXTRA_GO_LDFLAGS),$(error EXTRA_GO_LDFLAGS must be an unpatterned linker arg list beginning with '-'; Go package-pattern forms are unsupported),)-ldflags "$(_BUILD_STAMP_LDFLAGS)"

# Every target that owns a literal root-Makefile `go build -o` recipe is
# admitted through the Go-compatible GOFLAGS guard. tests/buildstamp compares
# this registry with the recipes so a new governed build cannot bypass it.
override _GOVERNED_BUILD_TARGETS := \
	health-check \
	build-absence-alarm \
	build-merge-health \
	build-reaper-e2e \
	build-routing-guard \
	build-stamp-test-probe \
	build-configure-settings \
	build-safe-push \
	build-safe-merge \
	build-safe-rebase \
	build-token-refresher \
	build-safe-pr \
	build-src-recovery \
	build-safe-unlock \
	build-jaeger-health \
	build-otel-local \
	build-bead-pr-sync \
	build-bead-pr-guard \
	build-codex-hook-json \
	build-bead-close-guard \
	build-drift-check \
	build-babysit-prs \
	build-external-pr-reviewer \
	build-mergeloop \
	build-resolve-review-threads \
	build-pr-blockers \
	build-merge-audit \
	build-dear-deploy \
	build-write-guards \
	build-bumblebee \
	build-pr-linkify \
	build-fd-pressure \
	build-gopls-watchdog \
	build-disk-watchdog \
	build-override-ledger-helper \
	build-override-audit-launchdaemon-installer \
	build-override-audit-systemd-installer \
	build-vroom-dispatch \
	build-vroom-mesh \
	build-vroom-prompt-gen \
	build-agm-job \
	build-src-health \
	build-burndown-maint \
	build-vroom-governor \
	build-agm \
	build-agm-mcp-server \
	build-engram-mcp \
	build-session-skill-extractor \
	test-affected
#
# Targets:
#   lint-specs              Validate EARS requirements in SPEC.md files
#   lint-skills             Validate every tracked skill and command prompt
#   plugin-verify-hashes    Verify AGM plugin command and skill content hashes
#   preflight               Fast local CI-parity gates: vet + build + AI skills + lint (~25s)
#   preflight-tests         preflight + go test (no -race) — quick sanity
#   preflight-race          preflight + go test -race — catch data races before push
#   preflight-full          race tests + ordinary performance SLAs + govulncheck
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
#   build-absence-alarm             Build absence-alarm: alarm on missing positive events
#   install-absence-alarm           Install absence-alarm to ~/go/bin
#   install-absence-alarm-launchagent   Stage the absence-alarm launch agent (10-min tick)
#   uninstall-absence-alarm-launchagent Remove the absence-alarm launch agent
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
#   build-merge-health      Build merge-health: merge-pipeline absence probe (jaeger-health sibling)
#   install-merge-health    Install merge-health to ~/go/bin
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

.PHONY: lint-specs preflight preflight-tests preflight-race preflight-full health-check install-preflight-hook install-post-merge-hook build-routing-guard install-routing-guard-hook act-validate act-lint act-test install-hooks test test-affected test-affected-print test-shell build-configure-settings install-configure-settings build-safe-push install-safe-push build-safe-merge install-safe-merge build-safe-rebase install-safe-rebase build-safe-pr install-safe-pr build-write-guards install-write-guards uninstall codegraph codegraph-all codegraph-install sync-main deepsec-incremental deepsec-staged install-deepsec-hook uninstall-deepsec-hook build-bumblebee bumblebee-install bumblebee-scan install-bumblebee-launchagent uninstall-bumblebee-launchagent structural-health structural-health-baseline build-src-recovery install-src-recovery build-safe-unlock install-safe-unlock build-jaeger-health install-jaeger-health build-bead-pr-sync install-bead-pr-sync install-bead-pr-sync-launchagent uninstall-bead-pr-sync-launchagent build-bead-pr-guard install-bead-pr-guard build-codex-hook-json install-codex-hook-json build-bead-close-guard install-bead-close-guard build-babysit-prs install-babysit-prs build-external-pr-reviewer install-external-pr-reviewer build-pr-linkify install-pr-linkify build-mergeloop install-mergeloop install-mergeloop-launchagent uninstall-mergeloop-launchagent build-drift-check install-drift-check drift-check drift-check-legacy deploy-status build-fd-pressure install-fd-pressure build-gopls-watchdog install-gopls-watchdog install-gopls-watchdog-launchagent uninstall-gopls-watchdog-launchagent uninstall-sandbox-gc-launchagent install-sandbox-gc-launchagent build-disk-watchdog install-disk-watchdog install-disk-watchdog-launchagent uninstall-disk-watchdog-launchagent build-override-audit-launchdaemon-installer install-override-audit-launchdaemon uninstall-override-audit-launchdaemon build-override-audit-systemd-installer install-override-audit-systemd uninstall-override-audit-systemd install-gobin-guard install-gobin-guard-launchagent uninstall-gobin-guard-launchagent build-vroom-dispatch install-vroom-dispatch build-vroom-mesh install-vroom-mesh build-agm-bus build-vroom-prompt-gen install-vroom-prompt-gen build-resolve-review-threads install-resolve-review-threads build-pr-blockers install-pr-blockers build-merge-audit install-merge-audit build-token-refresher install-token-refresher install-token-refresher-launchagent uninstall-token-refresher-launchagent build-dear-deploy install-dear-deploy dear-deploy-sync build-agm-job install-agm-job build-src-health install-src-health build-burndown-maint install-burndown-maint install-fd-limit-launchdaemon uninstall-fd-limit-launchdaemon build-otel-local install-otel-local otel-up build-vroom-governor install-vroom-governor build-agm install-agm build-agm-mcp-server install-agm-mcp-server build-engram-mcp install-engram-mcp
.PHONY: build-session-skill-extractor install-session-skill-extractor
.PHONY: build-absence-alarm install-absence-alarm install-absence-alarm-launchagent uninstall-absence-alarm-launchagent
.PHONY: build-merge-health install-merge-health
.PHONY: lint-skills
.PHONY: lint-instructions
.PHONY: lint-adrs
.PHONY: lint-headers
.PHONY: build-override-ledger-helper install-override-ledger-helper install-override-ledger-helper-locked
.PHONY: build-stamp-test-probe

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

# Publication gate: CI-parity race tests, ordinary (non-race) performance SLA
# enforcement, and govulncheck with the same allowlist as ci.yml. Slower but
# gives the highest confidence before pushing.
preflight-full:
	@./scripts/preflight.sh --full

# Build and run the codebase health auditor against this repo. Prints a
# markdown summary and exits 0 (healthy) / 1 (degraded) / 2 (critical).
# Pass ARGS to forward flags, e.g. `make health-check ARGS=--coverage` or
# `make health-check ARGS="--json-out health.json --md-out health.md"`.
# The scheduled .github/workflows/health-check.yml runs the same binary.
health-check:
	@mkdir -p build
	@GOWORK=off go build $(BUILD_STAMP_FLAGS) -o build/repo-health ./cmd/repo-health
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
	@mkdir -p build && go build $(BUILD_STAMP_FLAGS) -o build/routing-guard ./cmd/routing-guard && \
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

# Test-only executable seam for proving the runtime values injected by the
# governed Make build path. Callers must choose an output outside the repo.
build-stamp-test-probe:
	@test -n "$${_BUILD_STAMP_TEST_OUTPUT}" || { echo "BUILD_STAMP_TEST_OUTPUT is required" >&2; exit 2; }
	go build $(BUILD_STAMP_FLAGS) -o "$${_BUILD_STAMP_TEST_OUTPUT}" ./tests/buildstamp/testdata/probe/

# Run only the integration tests whose packages (or their transitive
# dependencies) changed vs. origin/main. See cmd/test-affected and
# docs/adr/ADR-024 for the algorithm and trust boundaries.
#
# Safety nets baked into the selector: go.mod / go.sum / Makefile /
# .github/workflows / the selector itself fall back to a full run, so
# this target is safe to default to locally before pushing.
test-affected:
	@tmp_bin=$$(mktemp); trap 'rm -f "$$tmp_bin"' EXIT; go build $(BUILD_STAMP_FLAGS) -o "$$tmp_bin" ./cmd/test-affected; "$$tmp_bin" --base=origin/main --tags=integration --run

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
	go build $(BUILD_STAMP_FLAGS) -o bin/configure-claude-settings ./cmd/configure-claude-settings/
	@echo "Built: bin/configure-claude-settings"

# Install configure-claude-settings to GOPATH/bin
install-configure-settings: build-configure-settings
	$(call install-go-bin,bin/configure-claude-settings)

# Build safe-push: a git-push wrapper that resets the credential helper chain
# to gh-only (never osxkeychain, which can hang on a headless GUI prompt) and
# refuses force-pushes to protected branches. See internal/safegit and
# vbonnet/engram-research
# retrospectives/2026-06-08-git-push-credential-hang.md.
build-safe-push:
	@echo "Building safe-push..."
	go build $(BUILD_STAMP_FLAGS) -o bin/safe-push ./cmd/safe-push/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/safe-merge ./cmd/safe-merge/
	@echo "Built: bin/safe-merge"

# Install safe-merge to GOPATH/bin so it is on PATH for every agent session.
install-safe-merge: build-safe-merge
	$(call install-go-bin,bin/safe-merge)

# Build safe-rebase: rebase feature branches onto main with safety checks.
# Refuses protected branches, aborts on conflict, optionally force-pushes
# + runs preflight in --auto mode.
build-safe-rebase:
	@echo "Building safe-rebase..."
	go build $(BUILD_STAMP_FLAGS) -o bin/safe-rebase ./cmd/safe-rebase/
	@echo "Built: bin/safe-rebase"

# Install safe-rebase to GOPATH/bin.
install-safe-rebase: build-safe-rebase
	$(call install-go-bin,bin/safe-rebase)

# Build token-refresher: single-owner, file-locked Claude Code OAuth refresher
# for the VROOM supervisor mesh. Keeps ~/.claude/.credentials.json fresh so
# expired access tokens stop killing the mesh (ce-rnpt / ce-f3e3).
build-token-refresher:
	@echo "Building token-refresher..."
	go build $(BUILD_STAMP_FLAGS) -o bin/token-refresher ./cmd/token-refresher/
	@echo "Built: bin/token-refresher"

# Install token-refresher to GOPATH/bin.
install-token-refresher: build-token-refresher
	$(call install-go-bin,bin/token-refresher)

# Wire token-refresher into the supervisor mesh (ce-cs3v): deploy the launchd
# idle-backstop that checks ~/.claude/.credentials.json every 30 minutes and
# refreshes only near access-token expiry, then print the single host-side,
# ask-gated activation step for you to run yourself.
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
	@echo "  Reload the idle backstop (bootout first -- a bare 'launchctl load' is a"
	@echo "  no-op against an already-loaded label and keeps running the STALE"
	@echo "  in-memory ProgramArguments, not what's on disk):"
	@echo "     launchctl bootout gui/\$$(id -u)/com.dear-agent.token-refresher 2>/dev/null || true"
	@echo "     launchctl bootstrap gui/\$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.token-refresher.plist"
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
	go build $(BUILD_STAMP_FLAGS) -o bin/safe-pr ./cmd/safe-pr/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/src-recovery ./cmd/src-recovery/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/safe-unlock ./cmd/safe-unlock/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/jaeger-health ./cmd/jaeger-health/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/otel-local ./cmd/otel-local/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/bead-pr-sync ./cmd/bead-pr-sync/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/bead-pr-guard ./cmd/bead-pr-guard/
	@echo "Built: bin/bead-pr-guard"

install-bead-pr-guard: build-bead-pr-guard
	$(call install-go-bin,bin/bead-pr-guard)

# Supplies only the fixed JSON filters used by attested Codex hook scripts as a
# static Go binary. The privileged install is digest-confirmed and root-staged
# so the unattended agent cannot replace either the executable or its runtime.
build-codex-hook-json:
	@echo "Building codex-hook-json..."
	@mkdir -p bin
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/codex-hook-json ./cmd/codex-hook-json/
	@echo "Built: bin/codex-hook-json"

install-codex-hook-json: build-codex-hook-json
	@set -eu; \
		test -t 0 || { echo "refusing non-interactive privileged Codex hook JSON helper installation" >&2; exit 2; }; \
		root_gid="$$(/usr/bin/id -g 0)"; \
		repo_root="$$(pwd -P)"; \
		artifact="$$repo_root/bin/codex-hook-json"; \
		helper="/usr/local/libexec/dear-agent-codex-hook-json"; \
		root_installer_path="$$repo_root/scripts/install-root-artifact.sh"; \
		root_installer="$$(/bin/cat "$$root_installer_path")"; \
		test -n "$$root_installer" || { echo "fixed privileged installer is empty" >&2; exit 2; }; \
		expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"; \
		expected_hash="$${expected_hash%% *}"; \
		expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"; \
		expected_installer_hash="$${expected_installer_hash%% *}"; \
		printf 'Reviewed Codex hook JSON helper SHA-256: %s\n' "$$expected_hash"; \
		printf 'Type that complete SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_hash; \
		printf 'Reviewed fixed privileged bootstrap SHA-256: %s\n' "$$expected_installer_hash"; \
		printf 'Type that complete SHA-256 to approve the fixed privileged command: '; \
		IFS= read -r confirmed_installer_hash; \
		test "$$confirmed_hash" = "$$expected_hash" || { echo "Codex hook JSON helper digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_installer_hash" = "$$expected_installer_hash" || { echo "privileged bootstrap digest confirmation did not match" >&2; exit 2; }; \
		privileged_child=""; \
		forward_privileged() { signal=$$1; status=$$2; trap - HUP INT TERM; test -z "$$privileged_child" || { /bin/kill "-$$signal" "$$privileged_child" 2>/dev/null || :; wait "$$privileged_child" || :; }; exit "$$status"; }; \
		trap 'forward_privileged HUP 129' HUP; trap 'forward_privileged INT 130' INT; trap 'forward_privileged TERM 143' TERM; \
		set +e; printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer" dear-agent-root-artifact-installer "$$artifact" "$$expected_hash" "$$root_gid" "$$helper" >/dev/null 2>&1 & privileged_child=$$!; \
		wait "$$privileged_child"; probe_status=$$?; privileged_child=""; set -e; \
		if test "$$probe_status" = 42; then echo "refusing passwordless sudo installer; fresh human authentication is required" >&2; exit 2; fi; \
		test "$$probe_status" = 1 || { echo "privileged installer probe failed unexpectedly (status $$probe_status)" >&2; exit 2; }; \
		printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer" dear-agent-root-artifact-installer "$$artifact" "$$expected_hash" "$$root_gid" "$$helper" & privileged_child=$$!; \
		wait "$$privileged_child"; privileged_child=""; trap - HUP INT TERM; \
		echo "Installed digest-bound operator-owned Codex hook JSON helper: $$helper"

# Enforces Definition of Done before bead closure: blocks `bd close` when
# referenced PRs are not yet merged. Used by the pretool-bead-close-guard hook.
# Usage: bead-close-guard --bead <id> [--repo owner/name] [--beads-dir /path]
build-bead-close-guard:
	@echo "Building bead-close-guard..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/bead-close-guard ./cmd/bead-close-guard/
	@echo "Built: bin/bead-close-guard"

install-bead-close-guard: build-bead-close-guard
	@set -eu; \
		test -t 0 || { echo "refusing non-interactive privileged bead-close guard installation" >&2; exit 2; }; \
		root_gid="$$(/usr/bin/id -g 0)"; \
		repo_root="$$(pwd -P)"; \
		artifact="$$repo_root/bin/bead-close-guard"; \
		guard="/usr/local/libexec/dear-agent-bead-close-guard"; \
		root_installer_path="$$repo_root/scripts/install-root-artifact.sh"; \
		root_installer="$$(/bin/cat "$$root_installer_path")"; \
		test -n "$$root_installer" || { echo "fixed privileged installer is empty" >&2; exit 2; }; \
		expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"; \
		expected_hash="$${expected_hash%% *}"; \
		expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"; \
		expected_installer_hash="$${expected_installer_hash%% *}"; \
		printf 'Reviewed bead-close guard SHA-256: %s\n' "$$expected_hash"; \
		printf 'Type that complete SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_hash; \
		printf 'Reviewed fixed privileged bootstrap SHA-256: %s\n' "$$expected_installer_hash"; \
		printf 'Type that complete SHA-256 to approve the fixed privileged command: '; \
		IFS= read -r confirmed_installer_hash; \
		test "$$confirmed_hash" = "$$expected_hash" || { echo "bead-close guard digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_installer_hash" = "$$expected_installer_hash" || { echo "privileged bootstrap digest confirmation did not match" >&2; exit 2; }; \
		privileged_child=""; \
		forward_privileged() { signal=$$1; status=$$2; trap - HUP INT TERM; test -z "$$privileged_child" || { /bin/kill "-$$signal" "$$privileged_child" 2>/dev/null || :; wait "$$privileged_child" || :; }; exit "$$status"; }; \
		trap 'forward_privileged HUP 129' HUP; trap 'forward_privileged INT 130' INT; trap 'forward_privileged TERM 143' TERM; \
		set +e; printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer" dear-agent-root-artifact-installer "$$artifact" "$$expected_hash" "$$root_gid" "$$guard" >/dev/null 2>&1 & privileged_child=$$!; \
		wait "$$privileged_child"; probe_status=$$?; privileged_child=""; set -e; \
		if test "$$probe_status" = 42; then echo "refusing passwordless sudo installer; fresh human authentication is required" >&2; exit 2; fi; \
		test "$$probe_status" = 1 || { echo "privileged installer probe failed unexpectedly (status $$probe_status)" >&2; exit 2; }; \
		printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer" dear-agent-root-artifact-installer "$$artifact" "$$expected_hash" "$$root_gid" "$$guard" & privileged_child=$$!; \
		wait "$$privileged_child"; privileged_child=""; trap - HUP INT TERM; \
		echo "Installed digest-bound operator-owned Codex hook guard: $$guard"
	$(call install-go-bin,bin/bead-close-guard)

# Detects deployment drift: deployed artifacts (Claude Code hooks, launchd
# plists, chezmoi files) whose source of truth in main no longer matches the
# copy on the host — a fix merged to git but never redeployed (PR #456). Cheap
# hash compare, no builds. See cmd/drift-check/README.md.
build-drift-check:
	@echo "Building drift-check..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/drift-check ./cmd/drift-check/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/babysit-prs ./cmd/babysit-prs/
	@echo "Built: bin/babysit-prs"

install-babysit-prs: build-babysit-prs
	$(call install-go-bin,bin/babysit-prs)

# Build external-pr-reviewer: configurable WRITE-permission PR review poller.
build-external-pr-reviewer:
	@echo "Building external-pr-reviewer..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/external-pr-reviewer ./cmd/external-pr-reviewer/
	@echo "Built: bin/external-pr-reviewer"

install-external-pr-reviewer: build-external-pr-reviewer
	$(call install-go-bin,bin/external-pr-reviewer)

# Build mergeloop: the Ralph Wiggum persistent PR-merge loop (ADR-029). Drives
# every open PR toward MERGED with zero human mechanics — rebases behind
# branches, spawns agents to fix CI/conflicts (--enable-agents), and delegates
# the squash-merge to safe-merge. Escalates only for policy blocks. See
# internal/mergeloop and ce-sbnd.
build-mergeloop:
	@echo "Building mergeloop..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/mergeloop ./cmd/mergeloop/
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

# Build resolve-review-threads: the sanctioned wrapper for GitHub review-thread
# reply and resolution mutations. Agents must use this instead of raw
# `gh api graphql` because the classifier blocks bare GraphQL mutations. The
# binary shells out to `gh api graphql` internally, so authentication uses the
# gh CLI token. Resolution requires evidence that the thread was answered; see
# `resolve-review-threads --help` for the subcommands rather than duplicating
# the usage catalogue here, where it goes stale.
build-resolve-review-threads:
	@echo "Building resolve-review-threads..."
	go build $(BUILD_STAMP_FLAGS) -o bin/resolve-review-threads ./cmd/resolve-review-threads/
	@echo "Built: bin/resolve-review-threads"

install-resolve-review-threads: build-resolve-review-threads
	$(call install-go-bin,bin/resolve-review-threads)

# Build pr-blockers: the deterministic PR merge-blocker classifier. Given a PR
# number it names the exact blocker set (draft, conflicts, failing/pending
# required checks, unresolved threads INCLUDING outdated, review decision,
# behind-base) and the exact fix for each, from GitHub's own merge state.
# Run it before investigating any stuck PR; never guess at a merge blocker.
build-pr-blockers:
	@echo "Building pr-blockers..."
	go build $(BUILD_STAMP_FLAGS) -o bin/pr-blockers ./cmd/pr-blockers/
	@echo "Built: bin/pr-blockers"

install-pr-blockers: build-pr-blockers
	$(call install-go-bin,bin/pr-blockers)

# Build merge-audit: safe-merge P6 detection tier. Weekly cross-repo sweep for
# unresolved-threads-at-merge, checks-incomplete-at-merge, direct pushes,
# break-glass overrides, and ruleset drift. Files a P1 bead per violation.
build-merge-audit:
	@echo "Building merge-audit..."
	go build $(BUILD_STAMP_FLAGS) -o bin/merge-audit ./cmd/merge-audit/
	@echo "Built: bin/merge-audit"

install-merge-audit: build-merge-audit
	$(call install-go-bin,bin/merge-audit)

build-merge-health:
	@echo "Building merge-health..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/merge-health ./cmd/merge-health/
	@echo "Built: bin/merge-health"

install-merge-health: build-merge-health
	$(call install-go-bin,bin/merge-health)

# Build dear-deploy: the write-side counterpart to drift-check. It deploys host
# artifacts (launchd plists, Claude Code hooks) from deploy/manifest.yaml through
# the principle-9 atomic sequence (stage -> verify -> activate). There is no
# bypass flag (ADR-031); a failed deploy leaves the prior artifact untouched.
build-dear-deploy:
	@echo "Building dear-deploy..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/dear-deploy ./cmd/dear-deploy/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/pretool-fs-write-guard ./cmd/pretool-fs-write-guard/
	go build $(BUILD_STAMP_FLAGS) -o bin/pretool-bash-write-guard ./cmd/pretool-bash-write-guard/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/dear-agent-bumblebee ./cmd/dear-agent-bumblebee/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/pr-linkify ./cmd/pr-linkify/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/fd-pressure ./cmd/fd-pressure/
	@echo "Built: bin/fd-pressure"

install-fd-pressure: build-fd-pressure
	$(call install-go-bin,bin/fd-pressure)

build-gopls-watchdog:
	@echo "Building gopls-watchdog..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/gopls-watchdog ./cmd/gopls-watchdog/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/disk-watchdog ./cmd/disk-watchdog/
	@echo "Built: bin/disk-watchdog"

install-disk-watchdog: build-disk-watchdog install-agm
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

build-absence-alarm:
	@echo "Building absence-alarm..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/absence-alarm ./cmd/absence-alarm/
	@echo "Built: bin/absence-alarm"

install-absence-alarm: build-absence-alarm
	$(call install-go-bin,bin/absence-alarm)

install-absence-alarm-launchagent: install-absence-alarm install-jaeger-health install-merge-health
	@mkdir -p $(HOME)/Library/LaunchAgents
	@mkdir -p $(HOME)/.local/state/dear-agent
	@mkdir -p $(HOME)/.config/dear-agent
	@[ -f $(HOME)/.config/dear-agent/absence-alarm-pulses.json ] || \
		cp deploy/absence-alarm/pulses.json $(HOME)/.config/dear-agent/absence-alarm-pulses.json
	@sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.dear-agent.absence-alarm.plist \
		> $(HOME)/Library/LaunchAgents/com.dear-agent.absence-alarm.plist
	@if [ ! -f $(HOME)/.local/state/dear-agent/absence-alarm.heartbeat.json ]; then \
		printf '{"tick_time":"%s","results":[],"alarming":0}\n' "$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
			> $(HOME)/.local/state/dear-agent/absence-alarm.heartbeat.json; \
		echo "Seeded: $(HOME)/.local/state/dear-agent/absence-alarm.heartbeat.json"; \
	fi
	@echo "Staged: $(HOME)/Library/LaunchAgents/com.dear-agent.absence-alarm.plist"
	@echo "Review the pulse set: $(HOME)/.config/dear-agent/absence-alarm-pulses.json"
	@echo "Activate it yourself (ask-gated host action):"
	@echo "  launchctl bootstrap gui/$$(id -u) $(HOME)/Library/LaunchAgents/com.dear-agent.absence-alarm.plist"

uninstall-absence-alarm-launchagent:
	@launchctl bootout gui/$$(id -u)/com.dear-agent.absence-alarm 2>/dev/null || true
	@rm -f $(HOME)/Library/LaunchAgents/com.dear-agent.absence-alarm.plist
	@echo "Removed: com.dear-agent.absence-alarm launch agent"

# On Unix systems without macOS authopen, authorized uses append through this
# one-purpose root-owned helper. Installation is an explicit operator action:
# it requires a fresh interactive sudo challenge and installs a NOPASSWD rule
# only for the helper's exact path, never for AGM, tee, chmod, or a variable
# destination.
build-override-ledger-helper:
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/dear-agent-override-ledger-append ./cmd/override-ledger-append/

install-override-ledger-helper: build-override-ledger-helper build-agm build-agm-mcp-server
	@set -eu; \
		install_lock_platform="$$(uname -s)"; \
		case "$$install_lock_platform" in \
			Darwin) install_lock_path="/private/var/run"; install_lock_tool="/usr/bin/lockf"; install_lock_ancestry="/ /dev /dev/fd /usr /usr/bin /private /private/var $$install_lock_path" ;; \
			Linux) install_lock_path="/run"; install_lock_tool="/usr/bin/flock"; install_lock_ancestry="/ /usr /usr/bin $$install_lock_path" ;; \
			*) echo "authenticated ledger callers are unsupported on this platform" >&2; exit 2 ;; \
		esac; \
		for install_lock_ancestor in $$install_lock_ancestry; do test -d "$$install_lock_ancestor" && test ! -L "$$install_lock_ancestor" && test ! -w "$$install_lock_ancestor"; case "$$install_lock_platform" in Darwin) install_lock_uid="$$(/usr/bin/stat -f '%u' "$$install_lock_ancestor")" ;; Linux) install_lock_uid="$$(/usr/bin/stat -c '%u' "$$install_lock_ancestor")" ;; esac; test "$$install_lock_uid" = 0; done; \
		for install_lock_executable in "$$install_lock_tool" /usr/bin/make; do test -f "$$install_lock_executable" && test ! -L "$$install_lock_executable" && test ! -w "$$install_lock_executable" && test -x "$$install_lock_executable"; case "$$install_lock_platform" in Darwin) install_lock_uid="$$(/usr/bin/stat -f '%u' "$$install_lock_executable")" ;; Linux) install_lock_uid="$$(/usr/bin/stat -c '%u' "$$install_lock_executable")" ;; esac; test "$$install_lock_uid" = 0; done; \
		exec 9<"$$install_lock_path"; \
		case "$$install_lock_platform" in \
			Darwin) DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED=1 exec "$$install_lock_tool" -k -t 0 /dev/fd/9 /usr/bin/make --no-print-directory install-override-ledger-helper-locked ;; \
			Linux) DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED=1 exec "$$install_lock_tool" --no-fork -n /proc/self/fd/9 /usr/bin/make --no-print-directory install-override-ledger-helper-locked ;; \
		esac

install-override-ledger-helper-locked:
	@set -eu; \
		test "$${DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED:-}" = 1 || { echo "refusing privileged ledger installation without the host transaction lock" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive privileged helper installation" >&2; exit 2; }; \
		operator_user="$$(id -un)"; \
		root_gid="$$(/usr/bin/id -g 0)"; \
		repo_root="$$(pwd -P)"; \
		artifact="$$repo_root/bin/dear-agent-override-ledger-append"; \
		agm_artifact="$$repo_root/bin/agm"; \
		companion_artifact="$$repo_root/bin/agm-mcp-server"; \
		agm_executable="$(HOME)/go/bin/agm"; \
		companion_executable="$(HOME)/go/bin/agm-mcp-server"; \
		agm_staging=""; \
		companion_staging=""; \
		agm_policy_artifact=""; \
		companion_policy_artifact=""; \
		transaction_nonce=""; \
		launcher_txdir=""; \
		agm_existed=0; companion_existed=0; launcher_activation_started=0; launcher_set_active=0; launcher_activation_complete=0; \
		root_installer_path="$$repo_root/scripts/install-override-ledger-root.sh"; \
		root_installer="$$(/bin/cat "$$root_installer_path")"; \
		test -n "$$root_installer" || { echo "fixed privileged installer is empty" >&2; exit 2; }; \
		/bin/mkdir -p "$(HOME)/go/bin"; \
		cleanup_staging() { status=$$1; trap - EXIT HUP INT TERM; set +e; /bin/rm -f "$$agm_staging" "$$companion_staging" "$$agm_policy_artifact" "$$companion_policy_artifact"; exit "$$status"; }; \
		trap 'cleanup_staging $$?' EXIT; trap 'cleanup_staging 129' HUP; trap 'cleanup_staging 130' INT; trap 'cleanup_staging 143' TERM; \
		agm_staging="$$(/usr/bin/mktemp "$$agm_executable.XXXXXX")"; \
		companion_staging="$$(/usr/bin/mktemp "$$companion_executable.XXXXXX")"; \
		/bin/cp "$$agm_artifact" "$$agm_staging"; \
		/bin/cp "$$companion_artifact" "$$companion_staging"; \
		/bin/chmod 0755 "$$agm_staging" "$$companion_staging"; \
		case "$$(uname -s)" in \
			Darwin) \
				/usr/bin/codesign -f -s - --options runtime "$$agm_staging"; \
				/usr/bin/codesign -f -s - --options runtime "$$companion_staging"; \
				;; \
			Linux) ;; \
			*) echo "authenticated ledger callers are unsupported on this platform" >&2; exit 2 ;; \
		esac; \
		agm_policy_artifact="$$(/usr/bin/mktemp "$$agm_executable.policy.XXXXXX")"; \
		companion_policy_artifact="$$(/usr/bin/mktemp "$$companion_executable.policy.XXXXXX")"; \
		/bin/cp "$$agm_staging" "$$agm_policy_artifact"; \
		/bin/cp "$$companion_staging" "$$companion_policy_artifact"; \
		/bin/chmod 0755 "$$agm_policy_artifact" "$$companion_policy_artifact"; \
		transaction_nonce="$$(/usr/bin/openssl rand -hex 32)"; test "$${#transaction_nonce}" = 64; \
		expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"; \
		expected_hash="$${expected_hash%% *}"; \
		expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"; \
		expected_installer_hash="$${expected_installer_hash%% *}"; \
		case "$$(uname -s)" in \
			Darwin) \
				caller_digest="$$(/usr/bin/codesign -dvvv "$$agm_staging" 2>&1 | /usr/bin/sed -n 's/^CDHash=//p' | /usr/bin/tr '[:upper:]' '[:lower:]')"; \
				test -n "$$caller_digest" || { echo "staged AGM has no kernel-verifiable code identity" >&2; exit 1; }; \
				caller_identity="darwin-cdhash:$$caller_digest"; \
				companion_digest="$$(/usr/bin/codesign -dvvv "$$companion_staging" 2>&1 | /usr/bin/sed -n 's/^CDHash=//p' | /usr/bin/tr '[:upper:]' '[:lower:]')"; \
				test -n "$$companion_digest" || { echo "staged AGM MCP companion has no kernel-verifiable code identity" >&2; exit 1; }; \
				companion_caller_identity="darwin-cdhash:$$companion_digest"; \
				;; \
			Linux) \
				caller_digest="$$(/usr/bin/sha256sum "$$agm_staging")"; \
				caller_digest="$${caller_digest%% *}"; \
				caller_identity="linux-sha256:$$caller_digest"; \
				companion_digest="$$(/usr/bin/sha256sum "$$companion_staging")"; \
				companion_digest="$${companion_digest%% *}"; \
				companion_caller_identity="linux-sha256:$$companion_digest"; \
				;; \
			*) echo "authenticated ledger callers are unsupported on this platform" >&2; exit 2 ;; \
		esac; \
		printf 'Reviewed helper SHA-256: %s\n' "$$expected_hash"; \
		printf 'Type that complete SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_hash; \
		test "$$confirmed_hash" = "$$expected_hash" || { echo "helper digest confirmation did not match" >&2; exit 2; }; \
		printf 'Reviewed installed AGM caller identity: %s\n' "$$caller_identity"; \
		printf 'Type that complete identity to bind privileged appends to these exact AGM bytes: '; \
		IFS= read -r confirmed_identity; \
		test "$$confirmed_identity" = "$$caller_identity" || { echo "AGM caller identity confirmation did not match" >&2; exit 2; }; \
		printf 'Reviewed installed AGM MCP companion identity: %s\n' "$$companion_caller_identity"; \
		printf 'Type that complete identity to permit launch-capability issuance from these exact companion bytes: '; \
		IFS= read -r confirmed_companion_identity; \
		test "$$confirmed_companion_identity" = "$$companion_caller_identity" || { echo "AGM MCP companion identity confirmation did not match" >&2; exit 2; }; \
		printf 'Reviewed fixed privileged installer SHA-256: %s\n' "$$expected_installer_hash"; \
		printf 'Type that complete SHA-256 to approve the fixed privileged command: '; \
		IFS= read -r confirmed_installer_hash; \
		test "$$confirmed_installer_hash" = "$$expected_installer_hash" || { echo "privileged installer digest confirmation did not match" >&2; exit 2; }; \
		privileged_child=""; \
		restore_launcher() { restore_live=$$1; restore_backup=$$2; restore_existed=$$3; if test "$$restore_existed" = 1; then if test -e "$$restore_backup" || test -L "$$restore_backup"; then /bin/mv -f "$$restore_backup" "$$restore_live"; else test -e "$$restore_live" || test -L "$$restore_live"; fi; else /bin/rm -f "$$restore_live"; fi; }; \
		cleanup_launchers() { status=$$1; trap - EXIT HUP INT TERM; set +e; cleanup_failed=0; if test "$$launcher_activation_started" = 1 && test "$$launcher_activation_complete" != 1; then restore_launcher "$$agm_executable" "$$launcher_txdir/agm" "$$agm_existed" || cleanup_failed=1; restore_launcher "$$companion_executable" "$$launcher_txdir/agm-mcp-server" "$$companion_existed" || cleanup_failed=1; fi; /bin/rm -f "$$agm_staging" "$$companion_staging" "$$agm_policy_artifact" "$$companion_policy_artifact"; test -z "$$launcher_txdir" || /bin/rm -rf "$$launcher_txdir"; test "$$cleanup_failed" = 0 || { echo "failed to restore the prior authenticated launcher set" >&2; status=1; }; exit "$$status"; }; \
		root_transaction_committed() { test "$$(/bin/cat /usr/local/libexec/dear-agent-override-ledger-install.receipt 2>/dev/null)" = "$$transaction_nonce"; }; \
		forward_privileged() { signal=$$1; status=$$2; trap - HUP INT TERM; set +e; test -z "$$privileged_child" || { /bin/kill "-$$signal" "$$privileged_child" 2>/dev/null || :; wait "$$privileged_child" || :; privileged_child=""; }; if test "$$launcher_set_active" = 1 && root_transaction_committed; then launcher_activation_complete=1; fi; cleanup_launchers "$$status"; }; \
		trap 'cleanup_launchers $$?' EXIT; \
		trap 'forward_privileged HUP 129' HUP; trap 'forward_privileged INT 130' INT; trap 'forward_privileged TERM 143' TERM; \
		set +e; printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer" dear-agent-override-ledger-installer "$$root_gid" "$$operator_user" "$$artifact" "$$expected_hash" "$$caller_identity" "$$companion_caller_identity" "$$agm_policy_artifact" "$$companion_policy_artifact" "$$transaction_nonce" >/dev/null 2>&1 & privileged_child=$$!; \
		wait "$$privileged_child"; probe_status=$$?; privileged_child=""; set -e; \
		if test "$$probe_status" = 42; then echo "refusing passwordless sudo installer; fresh human authentication is required" >&2; exit 2; fi; \
		test "$$probe_status" = 1 || { echo "privileged installer probe failed unexpectedly (status $$probe_status)" >&2; exit 2; }; \
		launcher_txdir="$$(/usr/bin/mktemp -d "$(HOME)/go/bin/.dear-agent-launchers.XXXXXX")"; launcher_activation_started=1; \
		if test -e "$$agm_executable" || test -L "$$agm_executable"; then agm_existed=1; /bin/mv "$$agm_executable" "$$launcher_txdir/agm"; fi; \
		if test -e "$$companion_executable" || test -L "$$companion_executable"; then companion_existed=1; /bin/mv "$$companion_executable" "$$launcher_txdir/agm-mcp-server"; fi; \
		/bin/mv -f "$$agm_staging" "$$agm_executable"; agm_staging=""; \
		/bin/mv -f "$$companion_staging" "$$companion_executable"; companion_staging=""; launcher_set_active=1; \
		printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer" dear-agent-override-ledger-installer "$$root_gid" "$$operator_user" "$$artifact" "$$expected_hash" "$$caller_identity" "$$companion_caller_identity" "$$agm_policy_artifact" "$$companion_policy_artifact" "$$transaction_nonce" & privileged_child=$$!; \
		set +e; wait "$$privileged_child"; install_status=$$?; set -e; \
		root_transaction_committed && launcher_activation_complete=1; privileged_child=""; \
		test "$$install_status" = 0 && test "$$launcher_activation_complete" = 1; \
		/bin/rm -f "$$agm_policy_artifact" "$$companion_policy_artifact"; agm_policy_artifact=""; companion_policy_artifact=""; \
		/bin/rm -rf "$$launcher_txdir"; launcher_txdir=""; \
		trap - EXIT HUP INT TERM; \
		echo "Installed digest-bound root-owned ledger helper, AGM and MCP companion caller identities, and exact sudoers rule for $$operator_user"

# Install the macOS audit under launchd's system domain without activating it.
# Both scheduler and executable are root-owned, so an unattended same-user
# agent cannot replace them or disable the job through its GUI launchd domain.
# Installation is an explicit, freshly authenticated operator action.
build-override-audit-launchdaemon-installer:
	@mkdir -p bin
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/dear-agent-override-audit-launchdaemon-installer ./agm/cmd/override-audit-launchdaemon-installer/

install-override-audit-launchdaemon: build-agm build-override-audit-launchdaemon-installer
	@set -eu; \
		test "$$(uname -s)" = "Darwin" || { echo "launchd audit installation is macOS-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit installation" >&2; exit 2; }; \
		operator_user="$$(/usr/bin/id -un)"; \
		case "$$operator_user" in *[!A-Za-z0-9._-]*|"") echo "unsupported operator account name" >&2; exit 2;; esac; \
		root_gid="$$(/usr/bin/id -g 0)"; \
		repo_root="$$(pwd -P)"; \
		audit_artifact="$$repo_root/bin/agm"; \
		helper_artifact="$$repo_root/bin/dear-agent-override-audit-launchdaemon-installer"; \
		plist_candidate="$$(/usr/bin/mktemp "$${TMPDIR:-/tmp}/dear-agent-override-audit.XXXXXX")"; privileged_child=""; \
		cleanup_plist_candidate() { status=$$1; trap - EXIT HUP INT TERM; /bin/rm -f "$$plist_candidate"; exit "$$status"; }; \
		forward_privileged() { signal=$$1; status=$$2; trap - HUP INT TERM; test -z "$$privileged_child" || { /bin/kill "-$$signal" "$$privileged_child" 2>/dev/null || :; wait "$$privileged_child" || :; privileged_child=""; }; cleanup_plist_candidate "$$status"; }; \
		trap 'cleanup_plist_candidate $$?' EXIT; \
		trap 'forward_privileged HUP 129' HUP; \
		trap 'forward_privileged INT 130' INT; \
		trap 'forward_privileged TERM 143' TERM; \
		root_installer_path="$$repo_root/scripts/install-override-audit-launchdaemon-root.sh"; \
		test -f "$$root_installer_path" || { echo "missing fixed privileged bootstrap: $$root_installer_path" >&2; exit 2; }; \
		root_installer="$$(/bin/cat "$$root_installer_path")"; \
		test -n "$$root_installer" || { echo "fixed privileged bootstrap is empty" >&2; exit 2; }; \
		/usr/bin/sed "s|__OPERATOR_USER__|$$operator_user|g" "$$repo_root/deploy/launchd/com.dear-agent.override-audit.plist" >"$$plist_candidate"; \
		/usr/bin/plutil -lint "$$plist_candidate" >/dev/null; \
		expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"; expected_audit_hash="$${expected_audit_hash%% *}"; \
		expected_plist_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$plist_candidate")"; expected_plist_hash="$${expected_plist_hash%% *}"; \
		expected_helper_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$helper_artifact")"; expected_helper_hash="$${expected_helper_hash%% *}"; \
		expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"; expected_installer_hash="$${expected_installer_hash%% *}"; \
		printf 'Reviewed audit executable SHA-256: %s\n' "$$expected_audit_hash"; \
		printf 'Reviewed rendered LaunchDaemon SHA-256: %s\n' "$$expected_plist_hash"; \
		printf 'Reviewed transaction helper SHA-256: %s\n' "$$expected_helper_hash"; \
		printf 'Reviewed fixed privileged bootstrap SHA-256: %s\n' "$$expected_installer_hash"; \
		printf 'Type the executable SHA-256 to approve these exact bytes: '; IFS= read -r confirmed_audit_hash; \
		printf 'Type the LaunchDaemon SHA-256 to approve these exact bytes: '; IFS= read -r confirmed_plist_hash; \
		printf 'Type the transaction helper SHA-256 to approve these exact bytes: '; IFS= read -r confirmed_helper_hash; \
		printf 'Type the bootstrap SHA-256 to approve the exact privileged command: '; IFS= read -r confirmed_installer_hash; \
		test "$$confirmed_audit_hash" = "$$expected_audit_hash" || { echo "audit executable digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_plist_hash" = "$$expected_plist_hash" || { echo "LaunchDaemon digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_helper_hash" = "$$expected_helper_hash" || { echo "transaction helper digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_installer_hash" = "$$expected_installer_hash" || { echo "privileged bootstrap digest confirmation did not match" >&2; exit 2; }; \
		installer_marker="dear-agent-override-audit-launchdaemon-installer"; set +e; \
		printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer" \
			"$$installer_marker" "$$helper_artifact" "$$expected_helper_hash" "$$root_gid" \
			"$$audit_artifact" "$$plist_candidate" "$$expected_audit_hash" "$$expected_plist_hash" >/dev/null 2>&1 & privileged_child=$$!; \
		wait "$$privileged_child"; probe_status=$$?; privileged_child=""; set -e; \
		if test "$$probe_status" = 42; then echo "refusing passwordless sudo installer; fresh human authentication is required" >&2; exit 2; fi; \
		test "$$probe_status" = 1 || { echo "privileged installer probe failed unexpectedly (status $$probe_status)" >&2; exit 2; }; \
		printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer" \
			"$$installer_marker" "$$helper_artifact" "$$expected_helper_hash" "$$root_gid" \
			"$$audit_artifact" "$$plist_candidate" "$$expected_audit_hash" "$$expected_plist_hash" & privileged_child=$$!; \
		wait "$$privileged_child"; privileged_child=""; \
		/bin/rm -f "$$plist_candidate"; plist_candidate=""; trap - EXIT HUP INT TERM; \
		echo "Installed digest-bound root-owned audit executable and system LaunchDaemon"; \
		echo "Review, activate, and monitor it yourself (ask-gated host actions):"; \
		echo "  sudo launchctl bootstrap system /Library/LaunchDaemons/com.dear-agent.override-audit.plist"; \
		echo "  log stream --predicate 'senderImagePath == \"/usr/bin/logger\"'"

uninstall-override-audit-launchdaemon:
	@set -eu; \
		test "$$(uname -s)" = "Darwin" || { echo "launchd audit removal is macOS-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit removal" >&2; exit 2; }; \
		repo_root="$$(pwd -P)"; root_uninstaller_path="$$repo_root/scripts/uninstall-override-audit-launchdaemon-root.sh"; \
		root_uninstaller="$$(/bin/cat "$$root_uninstaller_path")"; test -n "$$root_uninstaller" || exit 2; \
		expected_uninstaller_hash="$$(printf '%s' "$$root_uninstaller" | /usr/bin/openssl dgst -sha256 -r)"; expected_uninstaller_hash="$${expected_uninstaller_hash%% *}"; \
		printf 'Reviewed fixed privileged uninstaller SHA-256: %s\n' "$$expected_uninstaller_hash"; \
		printf 'Type that complete SHA-256 to approve the fixed privileged command: '; IFS= read -r confirmed_uninstaller_hash; \
		test "$$confirmed_uninstaller_hash" = "$$expected_uninstaller_hash" || { echo "privileged uninstaller digest confirmation did not match" >&2; exit 2; }; \
		set +e; printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_uninstaller" dear-agent-override-audit-launchdaemon-uninstaller >/dev/null 2>&1; probe_status=$$?; set -e; \
		if test "$$probe_status" = 42; then echo "refusing passwordless sudo uninstaller; fresh human authentication is required" >&2; exit 2; fi; \
		test "$$probe_status" = 1 || { echo "privileged uninstaller probe failed unexpectedly (status $$probe_status)" >&2; exit 2; }; \
		printf 'UNINSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_uninstaller" dear-agent-override-audit-launchdaemon-uninstaller; \
		echo "Removed the root-owned dangerous-override audit LaunchDaemon and executable"

# Install the Linux audit under the system manager without activating it. The
# template runs a root-owned AGM copy as the named unprivileged operator, so an
# unattended same-user agent cannot replace the executable or disable the timer
# through `systemctl --user`. Installation is an explicit, freshly
# authenticated operator action.
build-override-audit-systemd-installer:
	@mkdir -p bin
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/dear-agent-override-audit-systemd-installer ./agm/cmd/override-audit-systemd-installer/

install-override-audit-systemd: build-agm build-override-audit-systemd-installer
	@set -eu; \
		test "$$(uname -s)" = "Linux" || { echo "systemd audit installation is Linux-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit installation" >&2; exit 2; }; \
		command -v systemctl >/dev/null || { echo "systemctl is required" >&2; exit 2; }; \
		operator_user="$$(/usr/bin/id -un)"; \
		case "$$operator_user" in *[!A-Za-z0-9._-]*|"") echo "unsupported operator account name" >&2; exit 2;; esac; \
		root_gid="$$(/usr/bin/id -g 0)"; \
		repo_root="$$(pwd -P)"; \
		audit_artifact="$$repo_root/bin/agm"; \
		service_artifact="$$repo_root/agm/systemd/dear-agent-override-audit@.service"; \
		timer_artifact="$$repo_root/agm/systemd/dear-agent-override-audit@.timer"; \
		helper_artifact="$$repo_root/bin/dear-agent-override-audit-systemd-installer"; \
		privileged_child=""; \
		forward_privileged() { signal=$$1; status=$$2; trap - HUP INT TERM; test -z "$$privileged_child" || { /bin/kill "-$$signal" "$$privileged_child" 2>/dev/null || :; wait "$$privileged_child" || :; }; exit "$$status"; }; \
		trap 'forward_privileged HUP 129' HUP; \
		trap 'forward_privileged INT 130' INT; \
		trap 'forward_privileged TERM 143' TERM; \
		root_installer_path="$$repo_root/scripts/install-override-audit-systemd-root.sh"; \
		test -f "$$root_installer_path" || { echo "missing fixed privileged installer: $$root_installer_path" >&2; exit 2; }; \
		root_installer="$$(/bin/cat "$$root_installer_path")"; \
		test -n "$$root_installer" || { echo "fixed privileged installer is empty" >&2; exit 2; }; \
		expected_audit_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$audit_artifact")"; \
		expected_audit_hash="$${expected_audit_hash%% *}"; \
		expected_service_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$service_artifact")"; \
		expected_service_hash="$${expected_service_hash%% *}"; \
		expected_timer_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$timer_artifact")"; \
		expected_timer_hash="$${expected_timer_hash%% *}"; \
		expected_helper_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$helper_artifact")"; \
		expected_helper_hash="$${expected_helper_hash%% *}"; \
		expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"; \
		expected_installer_hash="$${expected_installer_hash%% *}"; \
		printf 'Reviewed audit executable SHA-256: %s\n' "$$expected_audit_hash"; \
		printf 'Reviewed systemd service SHA-256: %s\n' "$$expected_service_hash"; \
		printf 'Reviewed systemd timer SHA-256: %s\n' "$$expected_timer_hash"; \
		printf 'Reviewed privileged transaction helper SHA-256: %s\n' "$$expected_helper_hash"; \
		printf 'Reviewed fixed privileged bootstrap SHA-256: %s\n' "$$expected_installer_hash"; \
		printf 'Type the executable SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_audit_hash; \
		printf 'Type the service SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_service_hash; \
		printf 'Type the timer SHA-256 to approve these exact bytes: '; \
		IFS= read -r confirmed_timer_hash; \
		printf 'Type the helper SHA-256 to approve the privileged transaction logic: '; \
		IFS= read -r confirmed_helper_hash; \
		printf 'Type the bootstrap SHA-256 to approve the fixed privileged command: '; \
		IFS= read -r confirmed_installer_hash; \
		test "$$confirmed_audit_hash" = "$$expected_audit_hash" || { echo "audit executable digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_service_hash" = "$$expected_service_hash" || { echo "systemd service digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_timer_hash" = "$$expected_timer_hash" || { echo "systemd timer digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_helper_hash" = "$$expected_helper_hash" || { echo "privileged helper digest confirmation did not match" >&2; exit 2; }; \
		test "$$confirmed_installer_hash" = "$$expected_installer_hash" || { echo "privileged bootstrap digest confirmation did not match" >&2; exit 2; }; \
		installer_marker="dear-agent-override-audit-systemd-installer"; \
		set +e; \
		printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer" \
			"$$installer_marker" "$$helper_artifact" "$$expected_helper_hash" "$$root_gid" \
			"$$audit_artifact" "$$service_artifact" "$$timer_artifact" \
			"$$expected_audit_hash" "$$expected_service_hash" "$$expected_timer_hash" >/dev/null 2>&1 & privileged_child=$$!; \
		wait "$$privileged_child"; probe_status=$$?; privileged_child=""; \
		set -e; \
		if test "$$probe_status" = 42; then \
			echo "refusing passwordless sudo installer; fresh human authentication is required" >&2; \
			exit 2; \
		fi; \
		test "$$probe_status" = 1 || { echo "privileged installer probe failed unexpectedly (status $$probe_status)" >&2; exit 2; }; \
		printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer" \
			"$$installer_marker" "$$helper_artifact" "$$expected_helper_hash" "$$root_gid" \
			"$$audit_artifact" "$$service_artifact" "$$timer_artifact" \
			"$$expected_audit_hash" "$$expected_service_hash" "$$expected_timer_hash" & privileged_child=$$!; \
		wait "$$privileged_child"; privileged_child=""; trap - HUP INT TERM; \
		echo "Installed digest-bound root-owned audit executable and system unit templates"; \
		echo "Review, activate, and monitor them yourself (ask-gated host actions):"; \
		echo "  sudo systemctl enable --now dear-agent-override-audit@$$operator_user.timer"; \
		echo "  journalctl -t dear-agent-override-audit"

uninstall-override-audit-systemd:
	@set -eu; \
		test "$$(uname -s)" = "Linux" || { echo "systemd audit removal is Linux-only" >&2; exit 2; }; \
		test -t 0 || { echo "refusing non-interactive system audit removal" >&2; exit 2; }; \
		operator_user="$$(/usr/bin/id -un)"; case "$$operator_user" in *[!A-Za-z0-9._-]*|"") echo "unsupported operator account name" >&2; exit 2;; esac; \
		repo_root="$$(pwd -P)"; root_uninstaller_path="$$repo_root/scripts/uninstall-override-audit-systemd-root.sh"; \
		root_uninstaller="$$(/bin/cat "$$root_uninstaller_path")"; test -n "$$root_uninstaller" || exit 2; \
		expected_uninstaller_hash="$$(printf '%s' "$$root_uninstaller" | /usr/bin/openssl dgst -sha256 -r)"; expected_uninstaller_hash="$${expected_uninstaller_hash%% *}"; \
		printf 'Reviewed fixed privileged uninstaller SHA-256: %s\n' "$$expected_uninstaller_hash"; \
		printf 'Type that complete SHA-256 to approve the fixed privileged command: '; IFS= read -r confirmed_uninstaller_hash; \
		test "$$confirmed_uninstaller_hash" = "$$expected_uninstaller_hash" || { echo "privileged uninstaller digest confirmation did not match" >&2; exit 2; }; \
		set +e; printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_uninstaller" dear-agent-override-audit-systemd-uninstaller "$$operator_user" >/dev/null 2>&1; probe_status=$$?; set -e; \
		if test "$$probe_status" = 42; then echo "refusing passwordless sudo uninstaller; fresh human authentication is required" >&2; exit 2; fi; \
		test "$$probe_status" = 1 || { echo "privileged uninstaller probe failed unexpectedly (status $$probe_status)" >&2; exit 2; }; \
		printf 'UNINSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_uninstaller" dear-agent-override-audit-systemd-uninstaller "$$operator_user"; \
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
	go build $(BUILD_STAMP_FLAGS) -o bin/vroom-dispatch ./cmd/vroom-dispatch/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/vroom-mesh ./cmd/vroom-mesh/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/vroom-prompt-gen ./cmd/vroom-prompt-gen/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/agm-job ./cmd/agm-job/
	@echo "Built: bin/agm-job"

install-agm-job: build-agm-job
	$(call install-go-bin,bin/agm-job)

# Build src-health: canary that checks 7 ~/src repos for clean working tree,
# branch, and ahead/behind status. Used to soak the host dispatch loop during
# Phase A of ce-cd14.
build-src-health:
	@echo "Building src-health..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/src-health ./cmd/src-health/
	@echo "Built: bin/src-health"

install-src-health: build-src-health
	$(call install-go-bin,bin/src-health)

# Build burndown-maint: host-side bead-burndown maintenance tick (ce-cd14.2).
# Counts active burndown workers via agm session list, spawns at most 1 per
# tick up to --target (default 1). Run under agm-job for locking/escalation.
build-burndown-maint:
	@echo "Building burndown-maint..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/burndown-maint ./cmd/burndown-maint/
	@echo "Built: bin/burndown-maint"

install-burndown-maint: build-burndown-maint
	$(call install-go-bin,bin/burndown-maint)

# Build vroom-governor: system load + RAM monitor that pauses/resumes spawns
# and archives the newest worker on critical memory pressure (ce-lxdo).
# Interval and thresholds are configurable via flags.
build-vroom-governor:
	@echo "Building vroom-governor..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/vroom-governor ./cmd/vroom-governor/
	@echo "Built: bin/vroom-governor"

install-vroom-governor: build-vroom-governor
	$(call install-go-bin,bin/vroom-governor)

# Build the AGM CLI and its detached archive companion with one version stamp.
# A standalone `go install ./agm/cmd/agm` omits both the stamp and companion;
# prefer `make install-agm` so async archive compatibility remains coherent.
build-agm:
	@echo "Building agm + agm-reaper..."
	@mkdir -p bin
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/agm ./agm/cmd/agm/
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/agm-reaper ./agm/cmd/agm-reaper/
	@echo "Built: bin/agm bin/agm-reaper"

# Binaries the reaper E2E compose stack mounts. The seeder is a test fixture,
# not a shipped tool, so it is deliberately not part of build-agm/install-agm.
build-reaper-e2e: build-agm
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/seed-session ./agm/test/e2e/docker/cmd/seed-session/
	# Must be named `claude`: AGM matches the pane process COMM against the
	# harness, and Linux takes COMM from the executable file name.
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/claude ./agm/test/e2e/docker/cmd/mock-claude/
	@echo "Built: bin/seed-session bin/claude"

install-agm: build-agm
	$(call install-go-bin,bin/agm)
	$(call install-go-bin,bin/agm-reaper)
	@mkdir -p $(HOME)/.claude/hooks/session-start
	@install -m 755 agm/docs/hooks/session-start-agm.sh $(HOME)/.claude/hooks/session-start/agm-ready-signal
	@$(HOME)/go/bin/agm admin install-hooks

build-agm-mcp-server:
	@echo "Building agm-mcp-server..."
	@mkdir -p bin
	CGO_ENABLED=0 go build $(BUILD_STAMP_FLAGS) -o bin/agm-mcp-server ./agm/cmd/agm-mcp-server/
	@echo "Built: bin/agm-mcp-server"

install-agm-mcp-server: build-agm-mcp-server
	$(call install-go-bin,bin/agm-mcp-server)

# Build engram-mcp: Go Engram MCP server with verified beads writes (ce-ctsi).
# Supersedes the legacy Python server whose beads_create silently wrote to a
# JSONL file nothing reads. Requires BEADS_DB at runtime for beads tools.
build-engram-mcp:
	@echo "Building engram-mcp..."
	@mkdir -p bin
	go build $(BUILD_STAMP_FLAGS) -o bin/engram-mcp ./engram/cmd/engram-mcp/
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
	go build $(BUILD_STAMP_FLAGS) -o bin/session-skill-extractor ./cmd/session-skill-extractor/
	@echo "Built: bin/session-skill-extractor"

install-session-skill-extractor: build-session-skill-extractor
	$(call install-go-bin,bin/session-skill-extractor)

# Classify effective GOFLAGS once per governed dependency graph before any
# product recipe runs. The bootstrap ignores caller/persisted Go flags,
# workspace selection, and cross-compilation dimensions; the guard restores
# captured GOENV only inside its shell-free fallback query.
.PHONY: build-stamp-goflags-guard
$(_GOVERNED_BUILD_TARGETS): | build-stamp-goflags-guard
build-stamp-goflags-guard:
	@GOFLAGS=-buildvcs=false GOENV=off GOWORK=off GOOS= GOARCH= go run ./internal/buildstamp
