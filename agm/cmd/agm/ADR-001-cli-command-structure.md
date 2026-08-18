# ADR-001: Group AGM commands by resource

Status: Accepted

## Context

AGM has enough session, supervisor, administration, and diagnostic operations
that a flat command namespace creates collisions and makes discovery unstable.
The command tree is also consumed by generated skills, so aliases cannot be the
only source of truth.

## Decision

The Cobra tree groups operations under their owned resource. Session lifecycle
operations live below `agm session`; administration lives below
`agm admin`; other groups follow the same ownership rule. Compatibility
aliases may exist temporarily, but generated instructions and examples use the
canonical command path registered in Cobra.

The root command does not reinterpret an unknown first argument as a session
name. Session lookup is explicit at the command that owns it.

## Consequences

- Help output and generated skills share one discoverable hierarchy.
- New top-level names can be added without colliding with session names.
- Removed aliases require migrations, not silent reinterpretation.

## Evidence

- `main.go` and the command registration files beside it
- `parity/` tests and `../../test/bdd`
