# ADR-019: Derive A2A agent cards from session metadata

Status: Accepted (2026-03-24; verified 2026-07-17)

## Context

AGM session metadata is rich enough for local discovery but proprietary. Asking
operators to maintain a second card for every dynamic session guarantees drift.

## Decision

`agm/internal/a2a.GenerateCard` derives A2A-compatible names, descriptions, and
skills from AGM session metadata. A local file registry stores cards and
reconciles additions, updates, archived sessions, and orphans. HTTP publication
is a separate transport concern.

## Alternatives

Manual cards drift. A custom discovery schema duplicates A2A. Putting derived
cards in Dolt would make a cache look authoritative.

## Consequences

Skill inference is heuristic and local cards require reconciliation. A2A card
and registry tests verify serialization and lifecycle.
