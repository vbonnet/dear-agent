# ADR-002: Track retrieval at the service boundary

Status: Accepted

## Context

Tracking inside individual rankers misses local or fallback paths. Tracking only
in CLI commands misses non-CLI callers.

## Decision

The retrieval service records an access timestamp for each successfully parsed
engram path returned by search. It does not record query text, ranking metadata,
empty outcomes, or failed outcomes. Providers and rankers remain unaware of
tracking. Tracking failure does not change the retrieval result.

## Consequences

- All callers share one tracking boundary.
- Provider implementations remain focused on ranking.
- Access tracking remains best effort and cannot be used as retrieval authority.

## Evidence

- service implementation and tests in this package
- `../../../ecphory/telemetry_test.go`
