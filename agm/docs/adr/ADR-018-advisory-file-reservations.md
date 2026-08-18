# ADR-018: Advisory file reservations

Status: Accepted (2026-03-24; verified 2026-07-17)

## Context

Parallel agents can edit overlapping files. Hard locks are unsafe when an agent
can exit without releasing them, while no signal makes avoidable conflicts
invisible.

## Decision

`agm/internal/reservation` stores pattern-based, TTL-bound reservations in
`~/.agm/reservations.json`. Callers can reserve, check, and release. Conflicts
are advisory: policy may stop or proceed, but the storage primitive does not
deadlock work.

## Alternatives

OS locks do not span logical glob ownership and can be orphaned. Dolt locks add
storage coupling for ephemeral coordination. Branch separation alone does not
prevent two workers changing the same logical surface.

## Consequences

Participation is voluntary and stale intent survives until TTL expiry. Store
tests verify atomic updates, matching, expiry, and concurrent use.
