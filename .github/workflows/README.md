# GitHub Actions Workflows

## CI (`ci.yml`)

Required-status workflow. Core test jobs:

- **Build & Test** — runs `go test -race -count=1 ./...` on Ubuntu and
  macOS. The full unit-test surface, every PR, no exceptions.
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
