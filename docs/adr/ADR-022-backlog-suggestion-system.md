# ADR-022: Backlog Suggestion System

**Status**: Accepted
**Date**: 2026-05-15
**Context**: [ADR-015](ADR-015-signal-aggregator.md) and
[ADR-016](ADR-016-recommendation-mcp-server.md) shipped the *metric-driven*
"what should we work on next?" surface (git/lint/coverage/dep/security
signals → weighted score). They deliberately did not rank *declared work
items*. The VROOM Orchestrator (the COO supervisor — see
[/CONTEXT.md](../../CONTEXT.md) and
[docs/adr/ADR-002](ADR-002-vroom-execution-architecture.md); the
decision-trail concept is described there) needs the *task-driven*
counterpart: given the backlog of tickets, which one should be picked up next.
This ADR specifies that system. It is the DEAR **Define** artifact for the
framework-improvement item "backlog suggestion system" (ROADMAP cross-phase,
P2).

Builds on / aligns with:

- agm `ADR-023: Orchestrator Role` — the dispatch rules (dependency check,
  priority ordering, capacity check) this ranker mirrors so a suggestion is
  a faithful pre-dispatch decision.
- `pkg/vroom` — `EmitDispatched` / `TopicDecisionDispatched` is the
  Orchestrator decision-trail event a suggestion emits (`ADR-020`).
- `pkg/aggregator` — the *configurable-weights* scorer shape is reused as a
  design pattern (not a code dependency); backlog ranking is task-driven and
  intentionally separate from signal scoring.

---

## Context

The repo's backlog is declared, not derived: per-ticket tables in
`docs/workflow-engine/BACKLOG.md` and phase tables in `ROADMAP.md`. Today an
agent picking up work must read those markdown files by eye, resolve the
`Dep` column against `Status` columns, eyeball phase order, and judge effort
from the `Size` column. That is exactly the deterministic work the VROOM
Orchestrator (COO supervisor — see
[docs/adr/ADR-002](ADR-002-vroom-execution-architecture.md)) should do
before its agentic step. Three concrete gaps:

1. **No machine-readable backlog.** The ranking inputs (priority, deps,
   effort, status) exist only as GitHub-flavored markdown tables in two
   files with two *different* column layouts. Nothing parses them.
2. **No eligibility computation.** "Pick the lowest-numbered `pending`
   ticket whose deps are `done`" (BACKLOG.md § How to use) is stated as a
   human instruction, never enforced. Wildcard deps (`0.*`, `1.*`) make
   this error-prone by hand.
3. **No Orchestrator decision trail for pickup.** When work is chosen, no
   `vroom.decision.dispatched` event is emitted, so the pickup decision is
   invisible to the Overseer/Meta-Orchestrator and absent from the audit
   log — contradicting the dogfooding mandate in CLAUDE.md.

The metric surface (ADR-015/016) cannot fill this: its `Signal` is a
project-health observation (commits/week, lint count), not a work item with
dependencies and a status. Forcing tickets through `aggregator.Scorer` would
mean inventing fake "signal values" for tasks — a category error.

---

## Decision

Introduce `pkg/backlog` — a self-contained library that parses declared
backlog items, ranks them by Orchestrator dispatch rules plus effort, and
emits a VROOM dispatch decision when a suggestion is acted on — plus a thin
`cmd/backlog-suggest` CLI. The library has no SQLite, no network, and no
dependency on `pkg/aggregator`; its only non-stdlib dependency is
`pkg/vroom` for the Orchestrator-integration seam.

### D1. `Item` — the work-item model

`Item` mirrors the agm ADR-023 Orchestrator input contract
(`{id, title, priority, dependencies, ...}`) projected onto what the
markdown backlog actually declares:

```go
type Item struct {
    ID        string   // "0.1", "6.3", "X.1", "DEAR-X.5"
    Title     string
    Phase     int      // numeric prefix of ID; -1 for cross-phase (X.*)
    Priority  Priority // explicit HIGH/MED/LOW or P0..P3; Unset if absent
    Effort    Effort   // S=1, M=2, L=3; M when absent
    Status    Status   // Pending | InFlight | Blocked | Done
    Deps      []string // explicit IDs; a "N.*" entry is a phase wildcard
    Section   string    // enclosing "## Phase N ..." heading
    Files     string    // Files / Slot / Scope column, verbatim
}
```

Status parsing is tolerant: `done`, `done (#40)`, `done — note`,
strikethrough `~~...~~ DONE`, `in-flight (branch)`, `blocked (reason)`,
`pending`. Cells are stripped of backticks/asterisks before classification.

### D2. `Source` — pluggable, markdown by default

```go
type Source interface {
    Name() string
    Items(ctx context.Context) ([]Item, error)
}
```

One concrete implementation ships: `MarkdownSource`, a **header-aware**
GitHub-table parser (column meaning is resolved by normalized header name,
not position) so it reads *both* the 7-column BACKLOG.md layout and the
4-column ROADMAP.md `| id | Priority | Title | Slot |` layout from the same
code. The interface exists because the task brief explicitly anticipates
other sources (Asana, issue trackers) and the codebase strongly favors the
adapter pattern (`pkg/source`); no speculative second adapter is shipped —
that would be dead code (YAGNI).

### D3. `Ranker` — Orchestrator dispatch rules, made executable

Ranking reproduces agm ADR-023 § Task Dispatch as code:

1. **Eligibility (dependency + status gate).** A candidate must be
   `Pending` *and* every dependency must resolve to `Done`. A `N.*`
   wildcard dep requires every item in phase `N` to be `Done`. Unknown dep
   IDs are treated as unmet (fail safe). Ineligible items are kept but
   annotated with `Blockers`, never suggested.
2. **Priority ordering.** Explicit `Priority` dominates. When absent it is
   derived from phase order (earlier phase ⇒ higher priority), because
   phases are sequential and earlier phases unblock later ones.
3. **Dependency leverage.** Among equal priority, an item that unblocks
   more *direct* dependents ranks higher. This is the defensible analogue
   of "oldest first" (ADR-023 rule 3): it maximizes throughput by clearing
   chokepoints. Direct (not transitive) dependents — bounded, O(n²) worst
   case on a small backlog, and easy to reason about.
4. **Effort tiebreaker.** Smaller `Effort` first. Quick wins reduce WIP and
   unblock dependents sooner — the "estimated effort" axis the brief asked
   for.
5. **Determinism.** Final tiebreak is `Phase` then `ID` so the same
   backlog always yields the same order (testable, reviewable).

The blended `Score` is a transparent weighted sum over normalized
priority/leverage/effort with a `RankWeights` struct (zero value = sensible
defaults), echoing the configurable-weights pattern from
`aggregator.Scorer` without importing it.

### D4. `Suggester` — Source + Ranker + current context

```go
type Context struct {
    Phase    int  // -1 = any; otherwise restrict to this phase
    Capacity int  // max suggestions to return (Orchestrator max_parallel)
    MaxEffort Effort // 0 = no cap; else drop items above this size
}
```

`Suggester.Suggest(ctx, Context)` loads the source, ranks, applies the
context filters, and returns the top-`Capacity` eligible `Suggestion`s plus
every blocked item (with `Blockers`) so the CLI can explain *why* the
backlog is stuck, not just what is next.

### D5. Orchestrator integration — a synchronous dispatch decision

`OrchestratorNotifier` wraps a `vroom.EventPublisher` (the same seam
`selfimprove.VROOMNotifier` uses) and, on `Dispatch(Suggestion)`, publishes
`vroom.TopicDecisionDispatched` with a `DispatchedPayload`
(`TaskID=item.ID`, `Worker=` configured hint, `Rationale=` the ranking
reason) plus the standard `event_id`/`role`/`timestamp` envelope.

It publishes **synchronously**, deliberately *not* via `vroom.Emitter`:
`Emitter.emit` spawns a goroutine and a short-lived CLI would `os.Exit`
before it flushes, dropping the audit row. Synchronous publish is the
correct choice for a fire-once CLI and matches `selfimprove.VROOMNotifier`.
Emission is opt-in (`--emit-vroom`): `suggest` is a read; recording a
dispatch decision is a write to the decision trail and must be explicit.

### D6. CLI surface

`cmd/backlog-suggest` follows the `cmd/workflow-roles` pattern
(`main`→`run() int`, stdlib `flag`, exit 0/1/2, subcommand `flag.FlagSet`):

| Command   | Purpose                                                        |
|-----------|----------------------------------------------------------------|
| `list`    | Every parsed item with status/eligibility (`--json` supported) |
| `suggest` | Top-N eligible items + blocked-item explanations; `--emit-vroom` fires the dispatch decision for the top pick |

---

## Consequences

### Positive

- The "pick the next ticket" instruction in BACKLOG.md becomes executable
  and testable instead of a human convention.
- Work pickup gains a VROOM decision-trail row, satisfying the CLAUDE.md
  dogfdooding/audit mandate.
- Header-aware parsing means ROADMAP.md and BACKLOG.md (and future tables)
  are read by one parser with no per-file special-casing.
- Pure library (no DB/network) ⇒ fully unit-testable with table tests and
  testdata fixtures; meets the substrate test-quality bar.

### Negative

- The parser is coupled to GitHub-flavored markdown table syntax. A
  malformed table degrades to "no items" rather than erroring loudly; the
  CLI prints the parsed count so the operator notices a zero.
- Phase-derived priority is a heuristic; a genuinely urgent late-phase
  ticket needs an explicit `Priority` cell to outrank early-phase work.

### Neutral

- `Source` is an interface with one implementation. This is intentional
  shape for an anticipated extension point, not speculative generality.
- Ranking is separate from `aggregator.Scorer`. The two "what next?"
  surfaces (metric-driven, task-driven) stay independent on purpose.

## Implementation

- `pkg/backlog/`: `doc.go`, `item.go`, `source.go`, `rank.go`,
  `suggest.go`, `orchestrator.go` + `*_test.go` + `testdata/`.
- `cmd/backlog-suggest/`: `main.go`, `main_test.go`.
- ROADMAP.md cross-phase row updated to `done`.

## References

- [ADR-015: Signal Aggregator](ADR-015-signal-aggregator.md)
- [ADR-016: Recommendation MCP Server](ADR-016-recommendation-mcp-server.md)
- [/CONTEXT.md](../../CONTEXT.md) — VROOM vocabulary (Orchestrator role, decision trail)
- [docs/adr/ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md) — dispatch-rule + decision-trail source (supersedes agm ADR-020/023)
- `docs/workflow-engine/BACKLOG.md`, `ROADMAP.md` — the parsed sources
