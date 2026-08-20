# GitHub Actions Workflows

## Change detection (`changed-paths.yml`)

Reusable workflow (`on: workflow_call`) wrapping
[`cmd/changed-paths`](../../cmd/changed-paths), which owns the repo's path
taxonomy and emits boolean outputs (`go`, `agm`, `engram`, `deps`, `docs`,
`adr`, `global`).

Three rules make this safe. All three are enforced by
`cmd/changed-paths/workflow_contract_test.go`, because every one of them fails
by making a required check *absent* rather than red — and a skipped required
check satisfies branch protection exactly as well as a passing one.

1. **Never use workflow-level `on.<event>.paths` for anything that produces a
   required status check.** A workflow dropped by a path filter never creates a
   check run at all, so the required context sits at "Expected — Waiting for
   status to be reported" and the PR can never merge. A job skipped by an `if:`
   condition does report, with conclusion `skipped`, which GitHub accepts.

2. **Scope a required *matrix* job at the step level, never the job level.**
   GitHub does not expand `strategy.matrix` for a job whose job-level `if:` is
   false; the skipped check run carries the literal, unexpanded name. `Build &
   Test (ubuntu-latest)` would never be reported.

3. **Treat a failed detector as "everything is relevant".** An `if:` with no
   status function inherits an implicit `success()`, so a detector that dies on
   checkout would silently skip its consumers. Every scoped condition carries
   `!cancelled()` and an explicit `needs.changes.result != 'success'` clause.

Every output also fails safe to `true`: no base ref, a git error, a
non-`pull_request` event, or a change to a global input (`go.mod`, `go.sum`,
`Makefile`, `.github/**`, lint config) forces every consumer to run.
Go-relevance is a denylist, not an allowlist — a file counts as a build input
unless it is provably documentation, and Markdown that is `//go:embed`-ed or
hash-verified inside Build & Test is a build input too. See
[ADR-039](../../docs/adr/ADR-039-ci-path-scoping-and-gateway.md).

## CI (`ci.yml`)

Required-status workflow. Core test jobs:

- **Build & Test** — runs `go test -race -count=1 ./...` on Ubuntu and
  macOS. The job always runs so both required contexts are reported; its
  *steps* run on every PR that touches a build input, and skip on a pure
  documentation PR, which cannot change what it produces.
- **CI Gateway** — aggregate summary of the path-scoped jobs, and a **required
  status check** (`.github/rulesets/main.json`). Fails if any upstream job
  failed or was cancelled, and *also* fails if a job was skipped when the
  change set says it was relevant. That second assertion is what stops a
  mis-scoped filter from silently turning a required gate into a green tick —
  and it only bites because the gateway is itself required.
- **Integration Tests (affected)** — PR-only. Uses
  [`cmd/test-affected`](../../cmd/test-affected) to compute which
  `-tags=integration` test packages are reachable from the PR's diff and
  runs only those. Empty result = clean pass. See
  [ADR-028](../../docs/adr/ADR-028-smart-integration-test-selection.md)
  for the algorithm and trust boundaries; `make test-affected-print`
  shows the live decision locally.
- **AGM Codex Contracts** — runs the credential-free all-active-harness parity
  contract and the isolated source-built Codex create/list/send/kill/resume/
  archive lifecycle on every CI event, then enforces versioned statement-
  coverage floors for the backend, shared operations, state, and safety
  packages.
- **AGM Tagged Sweep** — on the daily schedule and manual dispatch, compiles and
  runs the full credential-free contract and integration graphs. Legacy
  provider-hosted Ginkgo scenarios remain explicit opt-in; portable contracts
  and the isolated source-built Codex lifecycle still run in this job.

Plus a `govulncheck` job that gates on known-vuln deps.

## AGM E2E Installation Tests (`agm-e2e-install.yml`)

Tests AGM installation from source across multiple Linux distributions.
Path-filtered to `agm/**` so it only runs when AGM itself changes.

No special setup or secrets required - the workflow runs automatically on push/PR.

### What Gets Tested

- **Ubuntu 22.04**: AGM installation from local source
- **Debian 12**: AGM installation from local source

Each test verifies:
1. Binary builds successfully
2. AGM command is available in PATH
3. `csm version` command works
4. Binary is installed to correct location (~/go/bin/csm)
