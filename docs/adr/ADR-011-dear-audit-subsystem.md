# ADR-011: Scheduled Repository Audit Subsystem

**Status**: Accepted (2026-06-16; was Proposed 2026-05-03)

> Filename keeps `dear-audit-subsystem` for inbound-link stability. The
> canonical title is "Scheduled Repository Audit Subsystem" so the old
> ambiguity does not propagate. Per
> [ADR-035](ADR-035-dear-terminology-disambiguation.md), this ADR concerns the
> workflow-engine lifecycle (Define / **Enforce** / Audit /
> **Resolve & Refine**), not Process DEAR (Define / **Execute** / Audit /
> **Retro**).

`pkg/workflow.Hooks.OnAudit` ([ADR-010](ADR-010-workflow-engine-architecture.md)
§D3) covers per-run audit — what happened during one workflow. It does
not cover per-repository, time-based audit: the recurring checks that
surface a test that quietly stopped exercising a path, a six-month-old
CVE in a dependency, a function nobody calls, a README link the upstream
moved. Today these are ad-hoc CI scripts: no shared lifecycle, no
history, no feedback into Define/Enforce, no cost-vs-importance discipline.

Add `pkg/audit/` as a first-class subsystem. It is the dual of the
workflow engine — same SQLite substrate, same audit-event spine, same
role/permission/budget primitives, but a different code path. Engine
runs work; audit runs checks.

- **D1. Checks are addressable, versioned, composable.** A `Check` is
  `Meta() CheckMeta` plus `Run(ctx, env) (Result, error)`. Stable IDs
  (`build`, `vuln.govulncheck`, `docs.dead-links`) in a singleton
  `Registry`. New checks ship as additive registrations.
- **D2. Declarative schedules, recommended defaults.** Each `CheckMeta`
  carries a recommended cadence (`daily / weekly / monthly / on-demand`);
  repos override in `.dear-agent.yml`. **The subsystem owns no clock** —
  it ships a generated GitHub Actions workflow + `workflow audit` CLI;
  the operator wires it to whatever scheduler they already use.
- **D3. Findings are structured, severity-ranked, de-duplicated.**
  `Fingerprint` (file + symbol + rule) is the load-bearing field — the
  same `unused export X.Foo` cannot inflate counts run-over-run. Severity
  is P0..P3, lifecycle is `open → acknowledged → resolved → reopened`.
  Trends are queries against the lifecycle, not raw counts.
- **D4. Remediation is a separate stage, gated per severity.** A check's
  job is to *find*. The runner calls a `Remediator` after all checks
  complete; strategies are `auto | pr | issue | noop`. P0/P1 default to
  auto where well-known; P2/P3 to issue. Keeps checks pure and
  trivially testable; lets the operator review `--dry-run` first.
- **D5. Refinement is an explicit output of every run.** After
  remediation, `Refiner`s look at the finding stream and propose
  amendments to Define and Enforce — a recurring `lint.go` finding
  becomes a proposed linter add; three `docs.dead-links` on the same
  domain become a CI link-check denylist. Proposals are suggestions,
  not auto-applied (HITL gate).
- **D6. One SQLite database, additive schema.** Three new tables on
  `runs.db`: `audit_findings`, `audit_runs`, `audit_proposals`. No
  existing column or index changes.
- **D7. Audits run as workflows.** A new `KindAuditCheck` node kind is
  the only new shape; permissions, budget, retry, and HITL all reuse the
  engine. An audit run shows up in `workflow status` and `workflow logs`.
- **D8. The subsystem owns its CLI: `workflow audit run | list | show |
  ack | resolve | propose | trends`.** Thin wrapper over `pkg/audit`.
- **D9. A check is wrong if it cannot be replayed offline.** Every
  built-in check ships a `testdata/` fixture + a `Mock` mode in the
  registry. A check that depends on network or shell without an offline
  test path is rejected at code review. Mirrors ADR-010 §D11.
- **D10. Severity is structural, not advisory.** P0/P1 can fail an audit
  run; downstream policy in `.dear-agent.yml > audits.severity-policy`
  decides what blocks. The subsystem reports truth; consumers decide.
- **D11. Dogfood on day one.** Acceptance criterion: dear-agent's daily
  audit runs cleanly and brain-v2's config lands. brain-v2 is polyglot
  (Go + Python + Docker); dear-agent exercises the single-module Go
  path. Together they prove the per-tree schema covers both shapes.
- **D12. No new external dependencies in v1.** Checks wrap `go build`,
  `go test`, `go vet`, `golangci-lint`, `govulncheck`, `git`, plain HTTP.
  Trivy / semgrep / cargo-audit are downstream adapters.

### Alternatives rejected

- **Use only `Hooks.OnAudit`.** Per-transition, not scheduled or
  repo-scoped.
- **Audits as Bash nodes.** Possible; loses fingerprint, severity,
  lifecycle, trend table.
- **Separate `audits.db`.** Splits the substrate. The whole point of
  ADR-010 is one queryable DB.
- **Build on Mend/Snyk.** Vendor lock-in for what *is* the substrate.
- **Run remediation inline.** Couples find and fix; loses dry-run.
- **Run everything on every commit.** Some checks (deep security, doc
  staleness) are too expensive. Cadence buckets exist for cost control.

### Bets

High stakes: fingerprints de-duplicate cleanly run-over-run; refinement
proposals are useful, not noise. Hedges: D9 offline tests assert
fingerprint stability; per-Refiner recurrence threshold + explicit
`propose` review CLI. Medium stakes: the schema is sufficient for v1;
audits-as-workflows is the right framing.

### Cross-references

- [ADR-009](ADR-009-work-item-as-first-class-substrate.md),
  [ADR-010](ADR-010-workflow-engine-architecture.md)
- `pkg/workflow/hooks.go` (existing `Hooks.OnAudit` surface)
- `pkg/workflow/schema.sql` (engine schema this ADR appends to)
