# ADR-009: WorkItem as First-Class Substrate (Separate from Session)

**Status**: Proposed (2026-05-02)

Today the "work item" is implicit. It lives as the free-text `goal` on a
session manifest, the subject of a bead, the artifact of an in-progress
Wayfinder phase, or the implicit topic of a tmux pane. This works for
single-session, single-agent work and breaks at every other boundary:

- **Cross-session handoff** — session A archives, session B picks up the
  same work; the link is reconstructable, not canonical.
- **Multi-agent coordination** — two agents working related pieces of the
  same job have no shared object to reason about.
- **Auditability** — "what happened to this work" requires walking
  manifests + beads + commits + phase artifacts and reconstructing the
  timeline.
- **External substrate integration** — GitHub Issues, Linear, Jira are all
  work-item systems; dear-agent has no native counterpart to map them
  onto, so integration becomes ad-hoc.

Introduce **WorkItem** as a first-class object, separate from Session. A
WorkItem represents the durable job; a Session represents the agent
process working on it. One WorkItem has zero-or-more Sessions over time,
at most one active. Verbs: `claim / release / block / unblock /
request_review / resolve / reopen / archive`. State machine and an
append-only event log per WorkItem. A pluggable `WorkItemProvider`
mirrors the harness-adapter pattern; initial providers are Local (Dolt
row + manifest), GitHub Issues, and Wayfinder phases as a board.

The cost: one more user-visible object, a migration story for existing
sessions, and provider drift (external schemas do not match exactly). The
payoff: "what happened to this work" becomes one event stream;
multi-agent coordination has a shared object; OpenAI Symphony-style
patterns (poll a board → spawn per-issue workspace) map naturally.

Acceptance is gated on a follow-up design doc with a concrete schema and
the four open questions resolved: where does Wayfinder fit (consumer of
WorkItems vs. implementation of the state machine), should beads be tied
to WorkItems or Sessions, what permissions govern external write-back
(closing a GitHub issue when a session resolves), and do we mint
dear-agent-local IDs or use provider-native IDs.

This ADR captures intent; the [workflow engine](ADR-010-workflow-engine-architecture.md)
is the natural home for the implementation (every node-execution is a
work-item record, every state transition is an audit event). The substrate
hypothesis ([docs/design/substrate-diagnostic.md](../design/substrate-diagnostic.md))
gives the per-component diagnostic this derives from.
