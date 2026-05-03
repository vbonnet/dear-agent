# ADR-009: Work Item as First-Class Substrate (Separate from Session)

**Status**: Proposed
**Date**: 2026-05-02
**Context**: Substrate Hypothesis research — see
[research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md](../../research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md)
and [docs/design/substrate-diagnostic.md](../design/substrate-diagnostic.md).

This ADR captures intent only. No code changes are required by accepting it.
Implementation is left for a follow-up ADR once the design has been pressure-
tested against real workflows.

## Context

dear-agent currently has a strong *session* substrate (manifest, state machine,
sandbox, audit log, Dolt rows) and a strong *workflow* substrate (Wayfinder
phases). It does not have a first-class **work item** — the durable thing that
represents *the job being done*, distinct from the agent process doing it.

Today, the "work item" is implicit. It lives as:

- The free-text `goal` field on a session manifest, or
- The subject of a bead, or
- The artifact of an in-progress Wayfinder phase, or
- The implicit topic of a tmux pane.

This works for single-session, single-agent work. It breaks down for:

- **Cross-session handoff.** Session A archives; session B picks up the same
  work. The link between them is reconstructable but not canonical.
- **Multi-agent coordination.** Two agents working on related but distinct
  pieces of the same work item have no shared object to reason about.
- **Auditability.** Asking "what happened to *this work*" requires walking
  multiple sources (manifests + beads + commits + phase artifacts) and
  reconstructing the timeline.
- **External substrate integration.** GitHub Issues, Linear, Jira are
  work-item systems. dear-agent has no native counterpart to map them onto,
  so integration becomes ad-hoc.

The substrate hypothesis (recorded in
[research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md](../../research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md))
argues that this is exactly the gap that issue trackers fill for human
coordination, and that agents need the same shape.

## Decision (Proposed)

Introduce **WorkItem** as a first-class object in dear-agent, separate from
**Session**. A WorkItem represents the durable job; a Session represents the
agent process currently working on it.

**Relationship:** one WorkItem has zero or more Sessions over time, but at
most one *active* Session at any moment. A Session always references its
WorkItem. A WorkItem outlives the Sessions that work on it.

The WorkItem object satisfies the substrate diagnostic:

| Question | WorkItem property |
|---|---|
| Records? | Stable ID, durable storage (Dolt row + manifest) |
| State machine? | Defined states (`open`, `claimed`, `in_progress`, `blocked`, `review`, `resolved`, `archived`); legal transitions enforced |
| Explicit ownership? | `assignee` field (a session ref or a human ref); transferred via named verb |
| Structural verbs? | `claim`, `release`, `block`, `unblock`, `request_review`, `resolve`, `reopen`, `archive` |
| Queryable history? | Append-only event log per WorkItem |

A WorkItem provider interface is introduced so the work-item layer is
pluggable, mirroring the harness adapter pattern:

```
type WorkItemProvider interface {
    Get(id string) (*WorkItem, error)
    List(filter Filter) ([]*WorkItem, error)
    Transition(id string, verb Verb, args ...) error
    History(id string) ([]Event, error)
}
```

Initial providers:

- **Local** — file/Dolt-backed, the default. Drop-in for users with no
  external work tracker.
- **GitHub Issues** — read/write to a configured repo's issues.
- **Wayfinder** — Wayfinder phase state exposed as a board.

Future providers (not in scope here): Linear, Jira, GitHub Projects.

## Consequences

### Positive

- **Symmetry across substrates.** dear-agent stops treating sessions as the
  unit of work and starts treating work items as the unit of work, with
  sessions as the execution mechanism. This matches how every external
  work-tracking system already thinks.
- **Cross-session continuity is canonical.** Resuming work after archival is
  a `claim` against the same WorkItem, not a manual reconstruction of state
  via fuzzy matching on session names.
- **Multi-agent coordination has somewhere to land.** Two agents working
  related work can hold sibling WorkItems with explicit `blocks` /
  `blocked_by` edges, instead of inferring relationships from sandbox path
  conflicts.
- **External integration becomes natural.** OpenAI Symphony's pattern (poll
  Linear board → spawn per-issue workspace) maps onto dear-agent as
  *poll WorkItemProvider → spawn session per claimed item*.
- **Auditability is per-job, not per-session.** "What happened to this
  feature" reads from one event stream.

### Negative / Costs

- **Migration burden.** Existing sessions / beads / phase artifacts have to
  be assigned to synthesized WorkItems, or the model has to support
  WorkItem-less sessions for legacy use. Both cost complexity.
- **Conceptual surface area grows.** Users learn one more object. The README
  has to explain why a WorkItem and a Session are not the same thing.
- **Provider drift.** External providers (GitHub, Jira) have schemas that
  don't match WorkItem's exactly; mapping layers will accumulate edge cases.
- **Premature abstraction risk.** If most users only ever use the Local
  provider, the adapter interface is overhead.

### Neutral

- **No immediate impact on AGM CLI surface.** WorkItem can be additive: new
  commands (`agm work list / claim / status / history`), no removal of
  existing session commands.
- **DEAR alignment.** WorkItem fits cleanly into DEAR — its state machine is
  a Define artifact, transitions are Enforce points, the event log is
  Audit, and the verb set covers Resolve.

## Open Questions

1. **Where does Wayfinder fit?** Is Wayfinder a *consumer* of WorkItems
   (each phase advances on a specific WorkItem) or an *implementation* of
   the WorkItem state machine (Wayfinder phases ARE the WorkItem states)?
   These are different products; the answer affects scope.
2. **Engram's relationship.** Beads are tied to sessions today. Should they
   be tied to WorkItems instead? Or both?
3. **External provider write-back.** When a session resolves a WorkItem
   backed by GitHub, does dear-agent close the GitHub issue? Under what
   permissions / approval?
4. **Identity collisions.** Stable IDs across providers — do we mint a
   dear-agent-local ID and map to provider-native IDs, or use the provider's
   native ID directly?

## Status

This ADR is **Proposed**, not Accepted. The intent is to record the
architectural direction surfaced by the substrate-hypothesis research so that
future design work has a reference point. Acceptance requires:

- A follow-up design doc with concrete schema, storage layout, and migration
  plan.
- Sign-off that the WorkItem abstraction is justified by real cross-session
  / multi-agent / external-integration scenarios, not theoretical ones.
- Resolution of the open questions above.

## References

- [research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md](../../research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md)
  — full research analysis
- [docs/design/substrate-diagnostic.md](../design/substrate-diagnostic.md)
  — the diagnostic questions and per-component scoring
- OpenAI Symphony spec (referenced in the source video) — example of an
  external system using a work-item board as the agent control plane
