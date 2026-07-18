# ADR-022: Backlog Suggestion System

Status: Accepted (2026-05-15)

[ADR-015](ADR-015-signal-aggregator.md) ranks projects by health metric
(commits/week, lint count, dep freshness). It cannot rank *declared
work items*: forcing tickets through `aggregator.Scorer` means inventing
fake "signal values" for tasks, which is a category error.

The VROOM Orchestrator (COO supervisor;
[ADR-002](ADR-002-vroom-execution-architecture.md),
[/CONTEXT.md](../../CONTEXT.md)) needs the task-driven counterpart.
Today the backlog is markdown tables in two files
(`docs/workflow-engine/BACKLOG.md` and `ROADMAP.md`) with two different
column layouts. "Pick the lowest-numbered `pending` whose deps are
`done`" is stated as a human instruction; wildcard deps (`0.*`, `1.*`)
make it error-prone by hand. Worse, when work is picked up, no
`vroom.decision.dispatched` event is emitted — so the pickup decision
is invisible to the Overseer and the Meta-Orchestrator.

Introduce `pkg/backlog/` — a self-contained library that parses declared
backlog items, ranks them by Orchestrator dispatch rules, and emits the
VROOM decision when a suggestion is acted on. Plus a thin
`cmd/backlog-suggest` CLI. No SQLite, no network; the only non-stdlib
dependency is `pkg/vroom` for the integration seam.

### Ranking rules, in order

1. **Eligibility.** `Pending` plus every dep resolves to `Done`. A `N.*`
   wildcard requires every phase-N item done. Unknown dep IDs are
   treated as unmet (fail safe). Ineligible items are kept but annotated
   with `Blockers` and never suggested.
2. **Priority.** Explicit `Priority` cell dominates. Absent, it is
   derived from phase order (earlier phases unblock later ones).
3. **Dependency leverage.** Among equal priority, more *direct*
   dependents wins — the defensible analogue of "oldest first" (agm
   ADR-023 rule 3). Bounded, O(n²) worst case on a small backlog.
4. **Effort tiebreaker.** Smaller `Effort` (S=1, M=2, L=3) first. Quick
   wins reduce WIP and unblock dependents sooner.
5. **Determinism.** Final tiebreak: `Phase` then `ID`. The same backlog
   always yields the same order.

The blended `Score` is a transparent weighted sum over normalized
priority/leverage/effort with a `RankWeights` struct (zero value =
sensible defaults), echoing `aggregator.Scorer`'s shape without
importing it. The two "what next?" surfaces stay independent on purpose.

### Why this shape

- **Header-aware markdown parser.** Column meaning resolves by
  normalized header name, not position, so the same code reads BACKLOG.md's
  7-column layout and ROADMAP.md's 4-column layout.
- **`Source` is an interface with one implementation.** Anticipated
  extension point (Asana, issue trackers), not speculative generality —
  no second adapter ships until something needs it.
- **Synchronous VROOM publish, not via `Emitter`.** A short-lived CLI
  would `os.Exit` before `Emitter.emit`'s goroutine flushes, dropping
  the audit row. `selfimprove.VROOMNotifier` is the precedent.
- **`--emit-vroom` is opt-in.** `suggest` is a read; recording a
  dispatch is a write to the decision trail and must be explicit.

CLI is two subcommands following the `cmd/workflow-roles` pattern:
`list` (every parsed item with status/eligibility, `--json` supported)
and `suggest` (top-N eligible + blocked-item explanations;
`--emit-vroom` fires the dispatch for the top pick).

### Consequences

The "pick the next ticket" instruction becomes executable and testable
instead of a human convention. Work pickup gains a VROOM decision-trail
row, satisfying the dogfooding/audit mandate. The parser is coupled to
GitHub-flavored markdown table syntax; a malformed table degrades to
"no items" rather than erroring loudly — the CLI prints the parsed
count so the operator notices a zero. Phase-derived priority is a
heuristic; a genuinely urgent late-phase ticket needs an explicit
`Priority` cell to outrank early-phase work.

### Cross-references

- [ADR-015: Signal Aggregator + Recommendation MCP](ADR-015-signal-aggregator.md)
- [ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md)
- [/CONTEXT.md](../../CONTEXT.md) — Orchestrator role, decision trail
- `docs/workflow-engine/BACKLOG.md`, `ROADMAP.md` — the parsed sources
