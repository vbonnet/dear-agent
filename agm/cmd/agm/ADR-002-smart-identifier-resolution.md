# ADR-002: Resolve session identifiers at the command boundary

Status: Accepted

## Context

Operators know sessions by UUID, exact name, or a sufficiently specific name
fragment. Requiring UUIDs for every action is costly, while silently selecting
one ambiguous session is unsafe.

## Decision

Resolution is command-specific today. Resume uses a layered resolver that
prefers exact IDs and names and may accept a unique name fragment. Capture uses
the exact session resolver, while storage-backed send paths use the Dolt
adapter's resolver. Each boundary reports its own ambiguity or not-found error;
callers must not assume that a fragment accepted by resume works everywhere.

Machine-readable output keeps the resolved canonical session identity so later
steps do not repeat a fuzzy lookup.

## Consequences

- Interactive use remains concise without sacrificing ambiguity safety.
- Resolver behavior must be tested at each command or storage boundary.
- Automation should persist canonical IDs after the first lookup.

## Evidence

- `resume.go`, `capture.go`, `../../internal/session/session.go`, and the Dolt adapter
- session command BDD scenarios in `../../test/bdd`
