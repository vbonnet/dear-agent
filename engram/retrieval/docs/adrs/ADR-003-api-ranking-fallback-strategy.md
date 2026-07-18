# ADR-003: Fall back locally when API ranking fails

Status: Accepted

## Context

Remote ranking improves some semantic queries but introduces credential,
network, quota, and provider failure modes.

## Decision

The service attempts configured API ranking only after it has local candidates.
If provider construction or ranking fails, it returns the local ranking with
degraded-path metadata rather than failing the search. Invalid request or source
errors remain errors and are not disguised as provider fallback.

## Consequences

- Retrieval stays available during provider outages.
- Result quality may degrade without changing the response contract.
- Callers can distinguish fallback from a fully provider-ranked result.

## Evidence

- retrieval service fallback tests
- `../../../ecphory/ranker.go`
