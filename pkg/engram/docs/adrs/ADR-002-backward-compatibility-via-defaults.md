# ADR-002: Default newly added Engram metadata when reading

Status: Accepted

## Context

Adding metadata fields must not make older valid Engram files unreadable.
Rewriting every file before a binary upgrade would create a deployment flag
day.

## Decision

The parser applies documented neutral defaults when optional fields are absent.
Writers emit the current schema, while migrations are reserved for changes that
cannot be represented safely by read-time defaults. Invalid explicit values are
not replaced silently.

## Consequences

- Older files remain readable after additive schema changes.
- Zero values require deliberate semantic review before use as defaults.
- Parser compatibility tests are required for each additive field.

## Evidence

- `../../parser.go`
- `../../parser_backward_compat_test.go`
