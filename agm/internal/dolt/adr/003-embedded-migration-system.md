# ADR-003: Embed ordered, checksummed Dolt migrations

Status: Accepted

## Context

AGM binaries must bring a workspace database to the schema they expect without
depending on a separately installed migration bundle. Rewriting an already
applied migration would make database history ambiguous.

## Decision

Migrations are compiled into the binary, ordered by component and version, and
recorded in `agm_migrations`. AGM computes a SHA-256 checksum from migration
SQL and refuses to continue when an applied version has different content.
Schema changes append a new migration rather than editing history.

## Consequences

- A binary and its migration set ship atomically.
- Applied migration content is immutable.
- Precondition and rollback behavior must be tested before release.

## Evidence

- `../migrations.go`
- migration and checksum tests beside it
