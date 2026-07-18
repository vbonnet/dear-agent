# ADR-001: Use Dolt as the authoritative AGM store

Status: Accepted

## Context

AGM needs transactional session and message state plus an auditable history that
can be inspected and repaired with database tooling. Maintaining SQLite and
JSONL as parallel authorities caused divergent state.

## Decision

Dolt is the authoritative operational store behind the SQL adapter. The adapter
owns schema access and exposes typed operations to callers. Exported event or
log files are projections, not competing state authorities.

## Consequences

- Operational state has one transactional owner and Dolt history.
- AGM depends on a reachable, compatible Dolt server.
- Backups and migrations must preserve database history and schema invariants.

## Evidence

- `../adapter.go`
- `../sessions.go`, `../messages.go`, and their tests
