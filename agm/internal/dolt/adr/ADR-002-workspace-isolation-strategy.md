# ADR-002: Isolate AGM workspaces by database

Status: Accepted

## Context

One AGM installation can operate several workspaces whose sessions, messages,
and reservations must not leak into one another.

## Decision

Each workspace normally selects a separate Dolt database. Session rows also
retain and filter by workspace as defense in depth. Child records such as
messages are keyed by globally unique message and session IDs and do not carry
their own workspace column, so configuring multiple workspaces to share one
`DOLT_DATABASE` is not an isolation boundary.

Explicit database configuration wins; otherwise the workspace name supplies
the database name. Tests use their own workspace and database.

## Consequences

- Cross-workspace isolation depends on distinct database selection.
- A wrong database selection fails isolation before row filtering is relevant.
- Schema migrations run independently for each workspace database.

## Evidence

- `../adapter.go`
- `../workspace_isolation_test.go`
