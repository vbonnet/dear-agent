# ADR-036: Wayfinder trace at the delivery boundary

Status: Accepted (2026-06-15; renumbered 2026-07-17)

## Context

Wayfinder phase files once remained dirty after transitions, and pull requests
could be opened without any lifecycle trace. That made the declared lifecycle
optional precisely where delivery evidence should be durable. The original
record also collided with the later ADR-035 terminology decision.

## Decision

- Wayfinder commits its status and history markers when a phase starts or
  completes. `GitIntegrator.CommitPhaseStart` and `CommitPhaseCompletion` own
  that boundary.
- `safe-pr` is the sanctioned PR create/close path. It requires an active
  `WAYFINDER-STATUS.md`, stamps the session into the PR, and records an audit
  event.
- Wayfinder project state lives outside the read-only `~/src` checkout.
- Exceptional inability to satisfy the gate is escalated; raw PR creation is
  not a second valid workflow.

## Alternatives

Ignoring marker files would keep working trees clean but discard the durable
phase trail. A soft reminder at PR time would preserve the untraceable path.
Both were rejected.

## Consequences

Phase transitions add small audit commits, and every delivery has a lifecycle
owner. The enforcement contract is covered by Wayfinder git integration tests
and `cmd/safe-pr` tests.
