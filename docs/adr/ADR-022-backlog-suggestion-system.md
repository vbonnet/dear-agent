# ADR-022: Backlog Suggestion System

Status: Deprecated (2026-07-17; Beads owns live work and VROOM dispatch)

[ADR-015](ADR-015-signal-aggregator.md) ranks projects by health metric
(commits/week, lint count, dep freshness). It cannot rank *declared
work items*: forcing tickets through `aggregator.Scorer` means inventing
fake "signal values" for tasks, which is a category error.

This record originally made repository Markdown tables the VROOM task source
and published a separate dispatch ledger. Direct Beads dispatch superseded both
choices. Beads now owns task identity, status, dependencies, and assignment.

`pkg/backlog` remains a format adapter for explicitly supplied Markdown
snapshots. `cmd/backlog-suggest` is read-only and requires `--files`; it has no
repository default and no VROOM emission path.

The legacy `agm task` commands and their `.agm/backlog.md` writer have been
removed. The orchestrator dashboard no longer reads that retired store.

### Ranking rules, in order

1. **Eligibility.** `Pending` plus every dep resolves to `Done`. A `N.*`
   wildcard requires every phase-N item done. Unknown dep IDs are
   treated as unmet (fail safe). Ineligible items are kept but annotated
   with `Blockers` and never suggested.
2. **Priority.** Explicit `Priority` cell dominates. Absent, it is
   derived from phase order (earlier phases unblock later ones).
3. **Dependency leverage.** Among equal priority, more *direct*
   dependents wins — the defensible analogue of the retired orchestrator's
   "oldest first" rule. Bounded, O(n²) worst case on a small backlog.
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
  normalized header name, not position, so explicitly supplied snapshots may
  use either supported table layout.
- **`Source` is an interface with one implementation.** Anticipated
  extension point (Asana, issue trackers), not speculative generality —
  no second adapter ships until something needs it.
CLI is two subcommands following the `cmd/workflow-roles` pattern:
`list` (every parsed item with status/eligibility, `--json` supported)
and `suggest` (top-N eligible plus blocked-item explanations). Both require
explicit `--files` input and perform no dispatch write.

### Consequences

The parser remains useful for inspecting archived or operator-supplied
GitHub-flavored Markdown. A malformed table degrades to
"no items" rather than erroring loudly — the CLI prints the parsed
count so the operator notices a zero. Phase-derived priority is a
heuristic; a genuinely urgent late-phase ticket needs an explicit
`Priority` cell to outrank early-phase work. Live work never flows from this
adapter into VROOM; Beads and direct dispatch own that boundary.

### Cross-references

- [ADR-015: Signal Aggregator + Recommendation MCP](ADR-015-signal-aggregator.md)
- [ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md)
- [/CONTEXT.md](../../CONTEXT.md) — direct Beads dispatch ownership
- [`cmd/backlog-suggest/SPEC.md`](../../cmd/backlog-suggest/SPEC.md)
