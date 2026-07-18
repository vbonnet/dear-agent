# ADR-001: Build retrieval indexes per search

Status: Accepted

## Context

Retrieval inputs can change between searches, while a shared mutable index
requires invalidation and synchronization that the service does not otherwise
need.

## Decision

The retrieval service builds the required in-memory index from the current
search sources for each request. Index state is request-scoped and is not a
process-wide cache.

## Consequences

- Each search observes its supplied source set.
- Requests do not share mutable index state.
- Repeated queries pay reconstruction cost; a future cache would require its
  own invalidation decision and evidence.

## Evidence

- `../../../ecphory/index.go`
- retrieval service tests
