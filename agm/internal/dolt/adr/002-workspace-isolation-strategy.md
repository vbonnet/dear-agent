# ADR-002: Isolate AGM workspaces by database

Status: Accepted

## Context

One AGM installation can operate several workspaces whose sessions, messages,
and reservations must not leak into one another.

## Decision

Each workspace selects a separate Dolt database. Stored rows also retain their
workspace identity as defense in depth, and query paths scope access through the
adapter rather than relying on callers to add filters.

Explicit database configuration wins; otherwise the workspace name supplies
the database name. Tests use their own workspace and database.

## Consequences

- Cross-workspace access requires an explicit aggregation path.
- A wrong database selection fails isolation before row filtering is relevant.
- Schema migrations run independently for each workspace database.

## Evidence

- `../adapter.go`
- `../workspace_isolation_test.go`
