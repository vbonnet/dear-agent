# Telemetry public API decision

Status: Accepted

## Context

External modules need to implement telemetry listeners, but Go prevents them
from importing Dear Agent's `internal/telemetry` package.

## Decision

`pkg/telemetry` is a minimal compatibility boundary that aliases the public
event, level, and listener types from `internal/telemetry`. Listener
registration and event production remain owned by the internal subsystem and
application composition roots.

The package does not duplicate telemetry implementation or promise a separate
lifecycle.

## Consequences

- External implementations share the exact internal type identities.
- Adding aliases expands the supported public API and requires review.
- Runtime behavior and thread-safety contracts remain defined and tested by
  `internal/telemetry`.

## Evidence

- `telemetry.go` and `telemetry_test.go`
- `../../internal/telemetry/`
