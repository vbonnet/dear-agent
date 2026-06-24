# BDD Scenario Catalog

Behavior-Driven Development (BDD) test scenarios for AGM. These scenarios make
AGM's SPEC invariants executable: each one is driven directly against the real
`internal/ops` and `internal/contracts` packages with `godog`.

---

## Overview

Scenarios are written in Gherkin and executed by `godog` via `TestFeatures`.
There is **no tag filter**: every `.feature` file under `test/bdd/features/`
runs on every build. A scenario whose steps are not implemented fails as
`undefined` rather than being skipped — so this catalog can never drift back
into listing tests that do not actually run.

**Location:** [`test/bdd/features/`](../test/bdd/features/)

**How to run:** See [test/bdd/README.md](../test/bdd/README.md)

> History: an earlier, larger catalog listed a multi-agent adapter suite
> (session lifecycle, agent selection, conversation persistence, etc.) marked
> "passing". Those feature files were never wired to step definitions and never
> ran — they were deleted in the "end BDD limbo" cleanup. Only the
> SPEC-invariant features below remain, because only they are actually
> enforced.

---

## Feature Files

### Trust Protocol

**File:** [`trust_protocol.feature`](../test/bdd/features/trust_protocol.feature)

**Drives:** `ops.TrustRecord` / `ops.TrustScore` / `ops.TrustHistory`

**Key scenarios:**
- Trust score is always clamped to `[0, 100]`.
- Base score for a new session is `50`.
- Trust events are append-only and chronologically ordered.
- `gc_archived` has zero score impact; `false_completion` is the heaviest penalty.
- Empty session names and invalid event types are rejected.

**Why this matters:** Orchestrator delegation decisions depend on trust scores
reflecting agent reliability within well-defined bounds.

---

### Scan Loop

**File:** [`scan_loop.feature`](../test/bdd/features/scan_loop.feature)

**Drives:** `ops.DefaultCrossCheckConfig` and the scan-loop SLO contracts.

**Key scenarios:**
- Auto-approve matches only the RBAC allowlist (`Read`/`Glob`/`Grep`, never
  `rm`/`sudo`).
- Well-known tmux sessions (`main`, `default`) are excluded from unmanaged checks.
- Health status escalates `healthy → warning → critical` by severity.
- Scan loop reads its thresholds from the SLO contracts (intervals, timeouts,
  capture depth, list limits).

**Why this matters:** The scan loop is the orchestrator's situational
awareness; its safety (allowlist) and cadence (SLO thresholds) must be pinned.

---

### Stall Detection

**File:** [`stall_detection.feature`](../test/bdd/features/stall_detection.feature)

**Drives:** stall-type invariants and stall-detection SLO contracts.

**Key scenarios:**
- Permission-prompt stalls are `critical` severity.
- Error messages are normalized (paths/line numbers stripped) before counting.
- Detector thresholds come from the SLO contracts (permission timeout,
  no-commit timeout, error-repeat threshold).
- Exactly three stall types exist: `permission_prompt`, `no_commit`, `error_loop`.

**Why this matters:** The multi-agent system only makes forward progress if
stalled sessions are detected and recovered against agreed thresholds.

---

### Harness Parity

**File:** [`harness_parity.feature`](../test/bdd/features/harness_parity.feature)

**Drives:** `state.Detector` / `state.CheckCanReceive`

**Key scenarios:**
- A Codex CLI composer pane is detected as `ready`.
- An idle Codex composer allows direct delivery.
- A Codex trust prompt is queued rather than treated as a sendable prompt.

**Why this matters:** AGM's delivery contract is harness-neutral. Codex CLI uses
different terminal chrome than Claude Code, but the orchestrator must still know
when Codex can safely receive input and when a menu prompt must not receive
injected text.

---

## Running

```bash
cd agm
make test-bdd          # godog feature tests (TestFeatures)
go test ./test/bdd/... # features + SPEC invariants + contract drift
```

CI runs this package on every PR via the root `ci.yml` "Build & Test" job
(`go test -race ./...`).

---

## Adding a scenario

See [test/bdd/README.md](../test/bdd/README.md#adding-a-scenario). In short: add
the scenario, implement every step in `steps/<name>_steps.go`, register the
step group in `main_test.go`, and run `go test ./test/bdd/...`. An
unimplemented step fails the build — that is the enforcement mechanism.

---

## Next steps

- **Run scenarios:** [test/bdd/README.md](../test/bdd/README.md)
- **Choose agent:** [AGENT-COMPARISON.md](AGENT-COMPARISON.md)
- **Troubleshoot:** [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
