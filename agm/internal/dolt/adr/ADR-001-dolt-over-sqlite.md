# ADR-001: Use Dolt as the authoritative AGM store

Status: Accepted

## Context

AGM needs transactional session and message state plus an auditable history that
can be inspected and repaired with database tooling. Maintaining SQLite and
JSONL as parallel authorities caused divergent state.

## Decision

Dolt is authoritative for the session and message data owned by its SQL
adapter. The adapter owns schema access and exposes typed operations to callers.
Exported event or log files are projections of that adapter-owned state.

This decision does not cover every AGM persistence boundary. In particular,
queued delivery state, attempts, and acknowledgments in `internal/messages`
are operationally authoritative in SQLite for the delivery daemon.

## Consequences

- Dolt-adapter state has one transactional owner and Dolt history.
- The SQLite delivery queue remains a separate, explicitly bounded authority.
- AGM depends on a reachable, compatible Dolt server.
- Backups and migrations must preserve database history and schema invariants.

## Evidence

- `../adapter.go`
- `../sessions.go`, `../messages.go`, and their tests
- `../../messages/queue.go`
