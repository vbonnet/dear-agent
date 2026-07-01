# ADR-011: Scheduled Repository Audit Subsystem

**Status**: Accepted (2026-06-16; was Proposed 2026-05-03)

> Filename keeps `dear-audit-subsystem` for inbound-link stability;
> the canonical title is "Scheduled Repository Audit Subsystem" so the
> "DEAR" ambiguity ([/CONTEXT.md](../../CONTEXT.md) § Known Terminology
> Collisions, 2b) does not propagate. Read "DEAR" below as the
> workflow-engine *code* lifecycle (Define / **Enforce** / Audit /
> **Resolve & Refine**), not the *process* retrospective (Define /
> **Execute** / Audit / **Retro**).

`pkg/workflow.Hooks.OnAudit` ([ADR-010](ADR-010-workflow-engine-architecture.md)
§D3) covers per-run audit — what happened during one workflow. It does
not cover per-repository, time-based audit: the recurring checks that
surface a test that quietly stopped exercising a path, a six-month-old
CVE in a dependency, a function nobody calls, a README link the upstream
moved. Today these are ad-hoc CI scripts: no shared lifecycle, no
history, no feedback into Define/Enforce, no cost-vs-importance discipline.

Add `pkg/audit/` as a first-class subsystem. It is the dual of the
workflow engine: engine runs work; audit runs checks. The shipped v1
uses the same SQLite-compatible substrate and can be pointed at the
same database, but it is exposed through a standalone `workflow-audit`
binary and defaults to `.dear-agent/audit.db`. Workflow integration is
provided by normal workflow YAML wrappers that invoke that binary.

- **D1. Checks are addressable, versioned, composable.** A `Check` is
  `Meta() CheckMeta` plus `Run(ctx, env) (Result, error)`. Stable IDs
  (`build`, `vuln.govulncheck`, `docs.dead-links`) in a singleton
  `Registry`. New checks ship as additive registrations.
- **D2. Declarative schedules, recommended defaults.** Each `CheckMeta`
  carries a recommended cadence (`daily / weekly / monthly / on-demand`);
  repos override in `.dear-agent.yml`. **The subsystem owns no clock**.
  The shipped surface is `workflow-audit run`; operators wire that
  command into cron, GitHub Actions, or the included workflow YAML
  wrappers.
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
  not auto-applied. The shipped v1 persists proposals in
  `audit_proposals`; a proposal review CLI is not implemented.
- **D6. Additive SQLite schema.** The audit schema creates three tables:
  `audit_findings`, `audit_runs`, `audit_proposals`. No existing
  workflow column or index changes. `workflow-audit --db` may point at
  `runs.db` when operators want one file, but the binary defaults to
  `.dear-agent/audit.db` and the v1 code does not require co-location
  with workflow state.
- **D7. Audits are workflow-compatible, not a new node kind.** The
  workflow engine has no `KindAuditCheck`. Audits run as the standalone
  `workflow-audit` command, and the repository ships YAML workflows that
  wrap that command in ordinary `bash` nodes when operators want audit
  runs to appear in `workflow status` and `workflow logs`.
- **D8. The shipped CLI is `workflow-audit`.** Current subcommands are
  `run | list | show | ack | resolve`. The `workflow audit ...`
  namespace, `propose`, and `trends` commands are not implemented in v1.
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
- **Only ad-hoc Bash checks.** Simple to schedule, but loses fingerprint,
  severity, lifecycle, and proposal persistence. The shipped workflow
  YAML wrappers use Bash only as a transport for `workflow-audit`; the
  audit model still lives in `pkg/audit`.
- **Require one physical `runs.db` in v1.** Operationally convenient for
  joins, but too coupled for the first audit CLI. The current default is
  `.dear-agent/audit.db`, with `--db` available for operators who want
  to co-locate the audit tables with workflow state.
- **Build on Mend/Snyk.** Vendor lock-in for what *is* the substrate.
- **Run remediation inline.** Couples find and fix; loses dry-run.
- **Run everything on every commit.** Some checks (deep security, doc
  staleness) are too expensive. Cadence buckets exist for cost control.

### Bets

High stakes: fingerprints de-duplicate cleanly run-over-run; refinement
proposals are useful, not noise. Hedges: D9 offline tests assert
fingerprint stability; per-Refiner recurrence thresholds keep proposal
volume bounded. Medium stakes: the schema is sufficient for v1; wrapping
`workflow-audit` in normal workflow YAML is enough integration before a
dedicated audit node kind exists.

### Cross-references

- [ADR-009](ADR-009-work-item-as-first-class-substrate.md),
  [ADR-010](ADR-010-workflow-engine-architecture.md)
- `pkg/workflow/hooks.go` (existing `Hooks.OnAudit` surface)
- `pkg/workflow/schema.sql` (engine schema this ADR appends to)
