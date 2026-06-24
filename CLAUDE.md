# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build everything
go build ./...

# Build specific binaries
go build -o build/agm ./agm/cmd/agm
go build -o build/engram ./engram/cmd/engram

# Run all tests (matches CI)
go test -race -count=1 ./...

# Run tests for one product
go test ./agm/...
go test ./engram/...
go test ./wayfinder/...

# Run a single test
go test -v ./agm/internal/ops/... -run TestSessionLifecycle

# Run tests without race detector (faster)
go test ./...

# Lint (5m timeout, merge-base regression-only)
golangci-lint run --timeout=5m ./...

# Fast local CI parity (~25s): vet + build + lint
make preflight

# Full CI parity: preflight + tests + race + govulncheck
make preflight-full

# Run only tests affected by your changes vs origin/main
make test-affected

# Validate EARS requirements in SPEC.md files
make lint-specs
```

Test isolation: engram tests that create sessions require `ENGRAM_TEST_MODE=1` and `ENGRAM_TEST_WORKSPACE=test`. Use `testutil.RequireTestMode(t)` to enforce this.

CI runs with `GOWORK=off` — there is no go.work file; everything is one root module.

## Architecture

Four products in one Go monorepo, sharing a single `go.mod`:

| Product | Directory | Purpose |
|---------|-----------|---------|
| **AGM** | `agm/` | Agent Gateway Manager — spawns/monitors/reaps AI agent sessions (tmux-backed) |
| **Engram** | `engram/` | Persistent memory with cue-based retrieval |
| **Wayfinder** | `wayfinder/` | 9-phase SDLC workflow engine |
| **Tools** | `cmd/`, `tools/` | 60+ standalone CLI utilities |

### Three API surfaces, one operations layer

CLI (Cobra), MCP server (JSON-RPC), and Claude Code Skills all route through the same business logic in `agm/internal/ops/`. An operation implemented once is available everywhere. `OpContext` provides dependency injection (storage, tmux client, config, output format).

### Harness adapter pattern

AGM supports multiple AI agent CLIs via adapters in `agm/internal/agent/` — each implements the `Agent` interface (`Start`, `Stop`, `SendKeys`, `GetUUID`, `ParseHistory`, `Translate`). Adding a new harness means implementing this interface; no changes to core ops needed.

### Agent state detection

Session state (READY/THINKING/PERMISSION_PROMPT/COMPACTING/OFFLINE) is detected via a priority chain: hook execution → tmux pane inspection → manual tracking.

### Sandbox isolation

Three pluggable providers in `internal/sandbox/`: OverlayFS (Linux), APFS cloned volumes (macOS), git worktree (fallback). Lifecycle tied to session lifecycle.

### Framework hierarchy (see CONTEXT.md for full vocabulary)

Wayfinder (planning) → VROOM (supervisory execution) → AGM (session tool) → DEAR (per-task retro loop). VROOM is *above* AGM — it drives AGM as a tool. Documents that conflate them are stale.

## Key Directories

- `agm/internal/ops/` — shared operations layer (all three API surfaces)
- `agm/internal/agent/` — harness adapters (Claude, Gemini, Codex, OpenCode)
- `agm/internal/session/` — session lifecycle and manifest management
- `internal/sandbox/` — copy-on-write filesystem isolation providers
- `pkg/` — shared public packages (importable externally)
- `internal/` — private packages
- `codegen/` — code generation framework (separate go.mod)
- `deploy/launchd/` — macOS launch agent plists
- `infra/` — Terraform IaC for GitHub repos/branch protection

## Conventions

- **Go only** — no Python for anything we own. Rust/TypeScript only with stated justification.
- **Conventional Commits** for commit messages.
- **Linting**: `.golangci.yml` uses `new-from-merge-base: origin/main` so only new regressions are reported. Existing violations are baselined and burned down incrementally.
- **Version injection**: binaries use ldflags `-X main.Version/GitCommit/BuildDate/BuiltBy`.
- **Go memory envelope**: `env/go-baseline.env` exports `GOMEMLIMIT=512MiB`, `GOMAXPROCS`, `GOGC` — all build/test/daemon targets inherit it.
- **Atomic wrappers**: unsafe command chains are wrapped in Go binaries that enforce safety by construction (e.g. `safe-push`, `safe-merge`, `safe-rebase`, `safe-pr`). Raw forms are denied via PreToolUse hooks.
- **PR creation**: `safe-pr create --wayfinder <dir>` only — raw `gh pr create` is denied by hook.
- **Temporal artifacts** (designs, retros, wayfinder runs) go to `~/src/engram-research`, never committed to this repo. Living docs (ARCHITECTURE.md, ADRs, CLAUDE.md) stay here.
- **Beads tracker**: always use `bd --db ~/beads/context-engine/.beads <subcommand>` — never bare `bd`.

## Project-Scoped Instructions

See `.claude/CLAUDE.md` for the full mandatory engineering principles, delegation rules, and operational protocols that govern agent behavior in this repo.
