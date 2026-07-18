# ADR-002: Resolve session identifiers at the command boundary

Status: Accepted

## Context

Operators know sessions by UUID, exact name, or a sufficiently specific name
fragment. Requiring UUIDs for every action is costly, while silently selecting
one ambiguous session is unsafe.

## Decision

Commands that accept a session identifier delegate to the shared session
resolution path. Resolution prefers an exact identity, permits a unique
human-readable match, and returns an explicit ambiguity or not-found error
otherwise. Commands do not embed their own lookup precedence.

Machine-readable output keeps the resolved canonical session identity so later
steps do not repeat a fuzzy lookup.

## Consequences

- Interactive use remains concise without sacrificing ambiguity safety.
- Resolution changes are tested once rather than per command.
- Automation should persist canonical IDs after the first lookup.

## Evidence

- `../../internal/session/session.go` and its tests
- session command BDD scenarios in `../../test/bdd`
