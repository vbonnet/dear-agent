# ADR-011: DEAR Audit Subsystem

**Status**: Proposed
**Date**: 2026-05-03
**Context**: Builds on
[ADR-009: Work Item as First-Class Substrate](ADR-009-work-item-as-first-class-substrate.md)
and
[ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md).
Closes the **A** of DEAR (Define, Enforce, **Audit**, Resolve & Refine):
phases 0–5 of the workflow engine cover Define and Enforce; ADR-010
hooks gave us the *call surface* (`OnAudit`, `OnResolve`) but no
*subsystem* that actually runs scheduled audits and feeds findings
back to Define / Enforce. This ADR is that subsystem.

---

## Context

The workflow engine carries a `Hooks.OnAudit` callback that fires on
every state transition (ADR-010 §D3). That covers **per-run** audit:
"what happened during this workflow execution?". It does **not** cover
**per-repository, time-based** audit — the recurring health checks that
let a project notice it is rotting:

- a test that quietly stopped exercising a code path
- a CVE in a dependency we last bumped six months ago
- a function that nobody calls anymore
- a `README` link that broke when the upstream site moved
- a doc that drifted from the code it described

Each of these is a Define/Enforce *gap*: they exist because no rule
exists yet to catch them, or because the rule exists but is silent
unless run on a schedule. The DEAR loop calls this **Audit**, and its
output is twofold:

1. **Findings** — concrete defects discovered now, ranked by severity,
   with enough metadata to remediate.
2. **Refinement** — proposed amendments to Define (what we promise)
   and Enforce (what we check) so the same class of finding cannot
   recur silently.

Without a dedicated subsystem, audit lives as scattered ad-hoc scripts
(CI cron jobs, manual `golangci-lint` runs, `npm audit` invocations).
Each runs in isolation, none store history, none feed back into
Define/Enforce, and the cost-vs-importance trade-off — which audits
deserve daily vs. weekly vs. monthly — is encoded only as folklore.

---

## Decision

Add a first-class **Audit subsystem** to dear-agent. It is the dual of
the workflow engine: where the engine runs **work** (one DAG per
ticket), the audit subsystem runs **checks** (a fleet of scheduled
inspections per repository). They share the same SQLite substrate, the
same audit-event spine, and the same role/permission/budget primitives,
but they do not share a code path.

The decisions below are binding through the first audit release and
the dogfooding rollout to dear-agent and brain-v2.

### D1. Audit checks are addressable, versioned, and composable

A **check** is a Go value satisfying the `audit.Check` interface:

```go
type Check interface {
    Meta() CheckMeta                       // id, kind, default schedule, severity ceiling
    Run(ctx context.Context, env Env) (Result, error)
}
```

`CheckMeta.ID` is a stable string (`build`, `test`, `lint.go`,
`vuln.govulncheck`, `dead-code.unused`, `deps.freshness`,
`docs.dead-links`, etc.). Checks are registered in a singleton
`Registry`. Workflows / configs reference them by id. New checks ship
as additive registrations; the existing config never has to change.

### D2. Schedules are declarative, set per-check, with recommended defaults

Each `CheckMeta` carries a **recommended cadence** (`daily`, `weekly`,
`monthly`, `on-demand`). A repo overrides any check's cadence in
`.dear-agent.yml`. The engine never *runs* a cron itself — it ships a
generated GitHub Actions workflow + a `workflow audit` CLI that the
operator wires to whatever scheduler they already use (cron, GH
Actions, AGM heartbeat, Cloud Scheduler). This keeps the subsystem
honest: *we describe schedules; we do not own the clock*.

The recommended defaults are:

| Cadence  | Cost guideline | Built-in checks at v1                                        |
|----------|----------------|--------------------------------------------------------------|
| daily    | < 5 min total  | `build`, `test`, `lint.*`, `vuln.govulncheck`                |
| weekly   | < 30 min       | `coverage`, `dead-code.unused`, `deps.freshness`, `docs.dead-links` |
| monthly  | < 2 hours      | `test-quality`, `complexity`, `docs.staleness`, `license`, `security.deep` |
| on-demand| —              | `release-readiness`, `migration-safety`                      |

These are *defaults*, not hardcoded constants — anything overridable
in `.dear-agent.yml`. The defaults are a strong signal about
cost-vs-value, not a contract.

### D3. Findings are structured, severity-ranked, and de-duplicated

Each check emits zero or more `Finding`s:

```go
type Finding struct {
    CheckID    string
    Fingerprint string          // stable hash (file + symbol + rule), used for de-dup
    Severity   Severity         // P0..P3
    Title      string
    Detail     string
    Path       string           // file path or URL
    Line       int              // optional
    Suggested  Remediation      // optional: command, patch, or config delta
    Evidence   map[string]any   // raw tool output, for debugging
}
```

`Fingerprint` is the load-bearing field. The store keys findings on
`(repo, fingerprint)` so the same `unused export X.Foo` does not
inflate counts every time the audit runs. A finding has a lifecycle
(`open → acknowledged → resolved → reopened`) tracked in SQLite. The
trend reports are queries against this lifecycle, not raw counts.

Severity is fixed at four levels:

| Severity | Meaning                                   | Default action       |
|----------|-------------------------------------------|----------------------|
| P0       | Build-breaking; security; data-corrupting | Block release; page  |
| P1       | Quality regression; new CVE; broken test  | Auto-remediate or PR |
| P2       | Drift; stale; minor inefficiency          | Track; batch fix     |
| P3       | Cosmetic; informational                   | Track only           |

### D4. Remediation is a separate stage, gated per-severity

A check's job is to **find**. Remediation is a *separate*
`Remediator` stage that the runner calls after all checks complete.
Each `Remediation` has a `Strategy`:

- `auto` — runner executes it (e.g. `golangci-lint run --fix`,
  `go mod tidy`, dependency-bot bump). P0/P1 default to `auto` when
  the strategy is well-known.
- `pr` — runner produces a patch, opens a draft PR, assigns review.
  Default for P1 when no auto strategy exists.
- `issue` — runner files a tracked issue (Beads / GitHub / Linear).
  Default for P2/P3.
- `noop` — runner records the finding and stops. Default for any
  finding without a remediation strategy.

The split keeps `Check` implementations pure: a check that emits a
finding never also writes to disk. That makes checks trivial to test
and lets the operator audit "what would change" via `workflow audit
--dry-run` before authorising remediation.

### D5. Refinement is an explicit output of every audit run

After remediation, the runner invokes `Refiner`s — small functions
that look at the finding stream and propose **amendments to Define
and Enforce**:

- A repeated `lint.go` finding for an unused-import rule that's not
  in `.golangci.yml` → propose adding the linter.
- A repeated `vuln.govulncheck` hit on a transitive dep we keep
  pinning → propose a CI gate that fails on that vuln class.
- Three `docs.dead-links` findings on the same upstream domain →
  propose a CI link-check rule with that domain on a denylist.

Refinement output is a list of `Proposal`s written to
`audit_proposals` (table below). They are **suggestions**, not auto
apply: a proposal landing requires human review (HITL gate). This is
the loop closure — every audit run can deepen Define and Enforce
without humans hand-noticing the pattern.

### D6. One SQLite database, additive to the engine schema

The audit subsystem reuses the engine's `runs.db` (per ADR-010 §D2)
and adds three tables:

```sql
CREATE TABLE audit_findings (
    finding_id   TEXT PRIMARY KEY,
    repo         TEXT NOT NULL,
    fingerprint  TEXT NOT NULL,
    check_id     TEXT NOT NULL,
    severity     TEXT NOT NULL CHECK (severity IN ('P0','P1','P2','P3')),
    state        TEXT NOT NULL CHECK (state IN ('open','acknowledged','resolved','reopened')),
    title        TEXT NOT NULL,
    detail       TEXT,
    path         TEXT,
    line         INTEGER,
    first_seen   TIMESTAMP NOT NULL,
    last_seen    TIMESTAMP NOT NULL,
    resolved_at  TIMESTAMP,
    evidence_json TEXT,
    UNIQUE (repo, fingerprint)
);
CREATE INDEX idx_audit_findings_state    ON audit_findings (repo, state);
CREATE INDEX idx_audit_findings_check    ON audit_findings (repo, check_id);
CREATE INDEX idx_audit_findings_severity ON audit_findings (repo, severity);

CREATE TABLE audit_runs (
    audit_run_id TEXT PRIMARY KEY,
    repo         TEXT NOT NULL,
    cadence      TEXT NOT NULL,            -- daily|weekly|monthly|on-demand
    started_at   TIMESTAMP NOT NULL,
    finished_at  TIMESTAMP,
    state        TEXT NOT NULL CHECK (state IN ('running','succeeded','failed','partial')),
    triggered_by TEXT,
    findings_new      INTEGER NOT NULL DEFAULT 0,
    findings_resolved INTEGER NOT NULL DEFAULT 0,
    findings_open     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_audit_runs_repo ON audit_runs (repo, started_at);

CREATE TABLE audit_proposals (
    proposal_id   TEXT PRIMARY KEY,
    audit_run_id  TEXT REFERENCES audit_runs(audit_run_id) ON DELETE CASCADE,
    target_layer  TEXT NOT NULL CHECK (target_layer IN ('define','enforce')),
    title         TEXT NOT NULL,
    rationale     TEXT NOT NULL,
    patch         TEXT,                    -- unified diff or YAML fragment
    state         TEXT NOT NULL CHECK (state IN ('proposed','accepted','rejected','expired')),
    proposed_at   TIMESTAMP NOT NULL,
    decided_at    TIMESTAMP,
    decided_by    TEXT,
    decision_note TEXT
);
```

These tables JOIN against the existing `runs` / `audit_events` tables
when needed (e.g. an audit run is also a workflow run, so its `run_id`
appears in `audit_events`). The schema is additive — no existing column
changes, no existing index changes.

### D7. Audits run as workflows; the audit runner is the same runner

An `audit` workflow is a regular `pkg/workflow` workflow with a
specific node kind: `KindAuditCheck`. The runner dispatches it
through the same execution path as `KindBash`, just with a typed
`AuditCheck` body instead of a shell command. This means:

- Permissions, budget, retry, HITL all work the same way.
- An audit run shows up in `workflow status`, `workflow logs`, and
  the existing inspector.
- A check that takes too long is killed by the same per-node budget
  meter.

`KindAuditCheck` is the only new node kind in this ADR. The body is
small:

```yaml
- id: govulncheck
  kind: audit_check
  audit_check:
    check_id: vuln.govulncheck
    config:                       # optional, check-specific
      include_test_files: true
```

`configs/workflows/audit-daily.yaml`,
`configs/workflows/audit-weekly.yaml`, and
`configs/workflows/audit-monthly.yaml` ship as starter templates.
A repo opts in by adding `audits:` to its `.dear-agent.yml` —
the schema is described in §5 below.

### D8. The audit subsystem owns its CLI: `workflow audit`

A new `cmd/workflow-audit` binary:

```
workflow audit run    [--cadence daily|weekly|monthly|all] [--check ID...] [--dry-run]
workflow audit list   [--state open|all] [--severity P0..P3] [--check ID]
workflow audit show   <finding-id>
workflow audit ack    <finding-id> [--note "..."]
workflow audit resolve <finding-id> [--note "..."]
workflow audit propose [--accept ID...] [--reject ID...]
workflow audit trends [--check ID] [--days 30]
```

The CLI is a thin wrapper over `pkg/audit`. The run path delegates to
`pkg/workflow.Runner` against a generated audit workflow; the read
paths are direct SQL.

### D9. A check is wrong if it cannot be replayed offline

Every built-in check ships with:

1. A unit test that runs the check against a `testdata/` fixture and
   asserts the finding count and fingerprints.
2. A `Mock` mode in the registry so an audit workflow can be
   `workflow dev`-shelled without invoking real `go vet` / `trivy`
   subprocesses.

This is a hard rule: a check that depends on hitting a network or a
shell binary without an offline test path is rejected at code review.
It mirrors ADR-010 §D11's "YAML round-trips" rule: audits round-trip
through testdata.

### D10. Severity is structural, not advisory

`P0/P1` findings can fail an audit run (`audit_runs.state = failed`).
Failing is observable to the rest of the system: AGM's heartbeat can
key off it; CI can refuse to merge; the workflow engine can refuse
to start a new high-cost run. Whether failure *blocks* anything is a
per-repo policy in `.dear-agent.yml > audits.severity-policy`. The
audit subsystem reports the truth; downstream consumers decide
whether to treat it as a blocker.

### D11. The audit subsystem dogfoods on day one

The acceptance criterion for this ADR is that **dear-agent itself
runs the daily audit cleanly** and **brain-v2 has its config
landed**. Brain-v2 is multi-language (Go submodules + Python +
Docker), so its audit config exercises per-tree configuration; the
dear-agent config exercises the single-module Go path. Together they
prove the schema covers both shapes.

### D12. No new external dependencies in v1

V1 ships with checks that wrap tools already present in the dev
environment (`go build`, `go test`, `go vet`, `golangci-lint`,
`govulncheck`, `git`, plain HTTP for link checking). `trivy`,
`semgrep`, `cargo-audit`, language-specific tools are downstream
adapters. The first cut must work on a stock Go install + the
linters already in `.golangci.yml`.

---

## Architecture diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                .dear-agent.yml > audits:                            │
│   schedule rules, severity policy, remediation overrides            │
└──────────────────────────┬──────────────────────────────────────────┘
                           │  loaded by
                           ▼
   ┌──────────────────────────────────────────────────────────────┐
   │                       pkg/audit                              │
   │                                                              │
   │   ┌──────────────┐   ┌──────────────┐   ┌──────────────┐     │
   │   │  Registry    │   │   Runner     │   │   Store      │     │
   │   │ (built-in +  │──▶│ orchestrates │──▶│ findings,    │     │
   │   │  user)       │   │ check fleet  │   │ runs,        │     │
   │   └──────────────┘   └──────┬───────┘   │ proposals    │     │
   │                             │           └──────┬───────┘     │
   │   ┌──────────────┐   ┌──────▼───────┐          │             │
   │   │  Check[]     │   │  Remediator  │          │             │
   │   │ build, test, │   │ auto|pr|     │          │             │
   │   │ lint, vuln,  │   │ issue|noop   │          │             │
   │   │ ...          │   └──────┬───────┘          │             │
   │   └──────────────┘          │                  │             │
   │                      ┌──────▼───────┐          │             │
   │                      │   Refiner    │──────────┤             │
   │                      │ define/      │          │             │
   │                      │ enforce      │          ▼             │
   │                      │ proposals    │   ┌──────────────┐     │
   │                      └──────────────┘   │   runs.db    │     │
   │                                         │ (shared with │     │
   │                                         │  workflow)   │     │
   │                                         └──────────────┘     │
   └─────────────────────────┬────────────────────────────────────┘
                             │ executes via
                             ▼
   ┌──────────────────────────────────────────────────────────────┐
   │             pkg/workflow.Runner (existing)                   │
   │   audit workflows are regular workflows; KindAuditCheck      │
   │   is one new node kind. Permissions, budget, HITL all reuse  │
   │   the existing infrastructure.                               │
   └──────────────────────────────────────────────────────────────┘
                             ▲
                             │
   ┌─────────────────────────┴────────────────────────────────────┐
   │                cmd/workflow-audit (CLI)                      │
   │   run | list | show | ack | resolve | propose | trends       │
   └──────────────────────────────────────────────────────────────┘
```

---

## What changes

| Layer                          | Change                                               |
|--------------------------------|------------------------------------------------------|
| `pkg/audit/` (new)             | Check / Finding / Severity / Remediation / Refiner   |
| `pkg/audit/checks/` (new)      | Built-in checks: build, test, lint, govulncheck      |
| `pkg/audit/store/` (new)       | SQLite store + 3 new tables (findings/runs/proposals)|
| `pkg/audit/config/` (new)      | `.dear-agent.yml > audits:` loader                   |
| `pkg/workflow/types.go`        | Additive: `KindAuditCheck` + `AuditCheckNode` body   |
| `pkg/workflow/runner.go`       | One dispatch case for `KindAuditCheck`               |
| `pkg/workflow/schema.sql`      | Three additive `CREATE TABLE IF NOT EXISTS` blocks   |
| `cmd/workflow-audit/` (new)    | CLI                                                  |
| `configs/workflows/audit-*.yaml` (new) | Three starter workflow templates             |
| `.dear-agent.yml` (this repo)  | Add an `audits:` section configuring the daily set   |
| `~/src/brain-v2/.dear-agent.yml` | Add an `audits:` section across its sub-trees      |
| `docs/adrs/ADR-011-...md`      | This file                                            |

## What does **not** change

- Existing `pkg/workflow` schema rows / indexes / state machine.
- Existing CLI surfaces (`workflow run`, `workflow status`, etc.).
- The `Hooks.OnAudit` callback — that is per-state-transition audit
  and remains the engine's runtime audit log. The new subsystem is
  **scheduled, repo-scoped audit**, not a replacement.
- `roles.yaml` — checks do not call LLMs in v1.
- The output-routing semantics of `.dear-agent.yml > output-dirs` and
  `forbidden-paths`. The new `audits:` key is additive.

---

## 5. `.dear-agent.yml > audits:` schema

```yaml
audits:
  # Severity → action policy. Optional; defaults shown.
  severity-policy:
    P0: { fail-run: true,  remediate: auto, notify: true  }
    P1: { fail-run: true,  remediate: pr,   notify: true  }
    P2: { fail-run: false, remediate: issue }
    P3: { fail-run: false, remediate: noop  }

  # Per-cadence enabled checks. Empty/missing = inherit defaults.
  schedule:
    daily:
      - check: build
      - check: test
        config: { race: true }
      - check: lint.go
      - check: vuln.govulncheck

    weekly:
      - check: coverage
        config: { min-percent: 60 }
      - check: dead-code.unused
      - check: deps.freshness
      - check: docs.dead-links

    monthly:
      - check: test-quality
      - check: complexity
        config: { gocyclo-threshold: 15 }
      - check: docs.staleness
      - check: license

  # Per-tree overrides — for monorepos with mixed languages.
  trees:
    - path: ./importers/whatsapp     # python subtree
      checks-add:
        - check: lint.python
        - check: vuln.pip-audit
      checks-remove:
        - check: lint.go
        - check: vuln.govulncheck
```

Every key is optional. A repo with no `audits:` block runs the
defaults from §D2. A repo with `audits: {}` runs nothing — useful
for archives or read-only mirrors.

---

## 6. Performance targets

| Operation                                    | Target            |
|----------------------------------------------|-------------------|
| Daily audit total wallclock (medium repo)    | < 5 minutes       |
| Single check timeout (default)               | 5 minutes         |
| `workflow audit list` (10K findings)         | P95 < 50 ms       |
| `workflow audit trends --days 30` (medium)   | P95 < 200 ms      |
| Finding fingerprint stability across runs    | 100% (test gate)  |

---

## Consequences

### Positive

- **Audit becomes a substrate concern, not folklore.** Findings have
  IDs, lifecycles, and history. "What rotted last week?" is a SQL
  query, not a search through CI logs.
- **The DEAR loop closes.** Refinement proposals tie audit findings
  back to Define/Enforce, so the system gets stricter over time
  without humans hand-noticing patterns.
- **Cost is visible.** Each cadence has a budget. A check that blows
  the budget is killed by the existing budget meter and surfaces as
  a finding against itself ("audit/govulncheck exceeded its 5-minute
  budget" — itself a P2).
- **Brain-v2 and dear-agent share a schema.** The same `.dear-agent.yml
  > audits:` block describes both. Per-tree overrides make the schema
  sufficient for polyglot repos.
- **The workflow engine gets dogfooded.** Audit workflows are the
  most repeated workflow on the system, so they will surface engine
  bugs (HITL stalls, budget edge cases) faster than user workflows.

### Negative / costs

- **Three new SQL tables to maintain.** Schema additions are forever.
  Hedged by additive-only design and `IF NOT EXISTS` migrations.
- **The check API is new surface.** Every built-in check is a small
  Go file but they accumulate; we have to keep the interface stable
  through v1.
- **Refinement may produce noise.** A naive Refiner that proposes
  every recurring finding as a new lint rule will overwhelm reviewers.
  Mitigated by a per-Refiner "minimum recurrence count" before
  proposing.
- **Brain-v2's per-tree config is non-trivial.** Polyglot repos pay a
  one-time cost to enumerate trees. We accept that cost as the price
  of supporting them at all.

### Neutral

- **Audits run as workflows, but `workflow audit` is its own CLI.**
  This is intentional: the operator's mental model is "checks", not
  "DAGs". The CLI hides the workflow plumbing. Power users can
  `workflow run configs/workflows/audit-daily.yaml` directly.
- **No scheduling daemon.** ADR-010 declined to own a clock; this
  ADR follows. Operators wire `workflow audit run --cadence daily`
  to whatever scheduler they already use.

---

## Bets, ranked by stakes

**High stakes:**

1. **Findings de-duplicate cleanly via fingerprint.** If fingerprints
   churn across runs, every audit looks like 100 new findings and the
   trend report becomes useless. Hedged by D9: every check has an
   offline test that asserts fingerprint stability.
2. **Refinement proposals are useful, not noise.** If reviewers
   reject every proposal, the loop never closes. Hedged by the
   per-Refiner recurrence threshold and the explicit `propose`
   review CLI.

**Medium stakes:**

3. **The `.dear-agent.yml > audits:` schema is sufficient for v1.**
   We may need a v2 with per-environment overrides (CI vs. local).
4. **Audits-as-workflows is the right framing.** An alternative is
   audits-as-their-own-thing with their own runner; we picked
   workflows-with-a-new-node-kind because it reuses budget/HITL/etc.

**Low stakes:** the exact cadence buckets (we may add `hourly` for
build/test); the exact severity rubric; whether `KindAuditCheck`
deserves further sub-typing.

---

## Alternatives considered

| Alternative                                | Why rejected                                                                                              |
|--------------------------------------------|-----------------------------------------------------------------------------------------------------------|
| **Use only existing `Hooks.OnAudit`.**     | Engine hook fires per-transition during a run. It cannot drive a scheduled, repo-scoped check fleet.       |
| **Audits as Bash nodes only.**             | Possible, but loses fingerprint, severity, lifecycle, and the trend table. We'd reinvent it badly.        |
| **Separate `audits.db`.**                  | Splits the substrate. The whole point of ADR-010 is one queryable DB. Three additive tables is the price. |
| **Build on a third-party tool (Mend, Snyk).** | Vendor lock-in for a feature that is the *substrate*. The point is to own the lifecycle, not outsource it. |
| **Run remediation inline inside Check.**   | Couples find and fix; makes checks impossible to test offline; loses the dry-run guarantee.               |
| **Run-everything-on-every-commit.**        | Some checks (deep security, doc staleness) are too expensive. Cadence buckets exist for cost control.     |

---

## Open questions

These are deferred to follow-up ADRs or v1.x point releases:

1. **HITL on remediation.** P0 auto-remediation (e.g. `go mod tidy`
   on a CVE) probably does not need a human. P1 PR-style does. Do
   we need a per-strategy HITL gate, or is it enough that PRs
   inherently require review?
2. **Cross-repo trend reporting.** Brain-v2 and dear-agent will both
   accumulate findings in their own `runs.db`. A workspace-level
   roll-up ("every repo I own, sorted by P0 count") is useful but
   needs a workspace-level DB. Punted to a workspace ADR.
3. **Refiner output format.** Patch + rationale is fine for
   `.golangci.yml`; harder for "add a hook to settings.json". v1
   ships only the lint-rule and CI-rule cases.
4. **Notification policy.** "P0 → notify: true" begs the question
   "notify whom, where?" Today this routes through the existing
   AGM Discord bot; a richer policy (Slack channel, email, paging)
   is downstream.
5. **Migration path for repos with existing audit jobs.** A
   `workflow audit migrate` command could read a `.github/workflows/`
   audit job and produce an `.dear-agent.yml > audits:` block. Not
   in v1.

---

## Implementation plan

V1 is intentionally narrow. Five tickets:

- **A0:** `pkg/audit` package skeleton + interfaces + tests.
- **A1:** SQLite store + schema additions + tests.
- **A2:** Built-in checks: `build`, `test`, `lint.go`, `vuln.govulncheck`.
   One Refiner: `LintGapRefiner`. One Remediator: `LintAutoFix`.
- **A3:** `cmd/workflow-audit` CLI with `run`, `list`, `show`, `ack`,
   `resolve`. `KindAuditCheck` node kind in `pkg/workflow`.
- **A4:** `.dear-agent.yml > audits:` config loader + apply to
   dear-agent and brain-v2.

After A4 the system is dogfooded. A5+ (coverage, dead-code, deps,
docs, monthly tier, refiner expansion) ship as additive PRs.

---

## Status

This ADR is **Proposed**, not Accepted.

Acceptance authorizes A0–A4 to land as one PR (the v1 cut) and
commits the project to D1–D12 above for the duration of v1.
Decisions D1, D3, D6, D7, D9 are particularly load-bearing — a
follow-up ADR is required to revise them. D2, D4, D5, D8, D10–D12
may be tuned in subsequent PRs without a new ADR.

Acceptance does NOT pre-authorize:

- New external dependencies beyond what already ships in the dev
  environment (per D12).
- Changes to existing `runs`, `nodes`, `node_attempts`,
  `audit_events`, or `approvals` tables.
- A workspace-level audit DB (open question 2).
- Notification routing beyond Discord + CLI (open question 4).

---

## References

- [ADR-009 — Work Item as First-Class Substrate](ADR-009-work-item-as-first-class-substrate.md)
- [ADR-010 — Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
- `pkg/workflow/hooks.go` (existing `Hooks.OnAudit` surface)
- `pkg/workflow/schema.sql` (existing engine schema; this ADR appends to it)
