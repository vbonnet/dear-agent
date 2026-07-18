# ADR-002: Track retrieval at the service boundary

Status: Accepted

## Context

Tracking inside individual rankers misses local or fallback paths. Tracking only
in CLI commands misses non-CLI callers.

## Decision

The retrieval service records query and result metadata after the retrieval
outcome is known. Providers and rankers remain unaware of tracking. Tracking
failure does not change the retrieval result, but it is observable to the
service.

## Consequences

- All callers share one tracking boundary.
- Provider implementations remain focused on ranking.
- Telemetry remains best effort and cannot be used as retrieval authority.

## Evidence

- service implementation and tests in this package
- `../../../ecphory/telemetry_test.go`
