# ADR-004: Use a tiered Engram retrieval pipeline

Status: Accepted

## Context

Engram queries range from exact known-memory lookups to semantic recall. Sending
every query to an external model adds latency, cost, and availability risk,
while local lexical matching alone misses useful context.

## Decision

Retrieval progresses from deterministic local candidates through local ranking
to an optional configured model reranker. Each tier may return a usable result
or fall back to the next available tier. Absence or failure of an external
provider does not disable local retrieval.

The command surface exposes the result and available explanation data; it does
not promise a particular external provider.

## Consequences

- Common queries remain local and available offline.
- Higher-cost ranking is explicit and optional.
- Ranking quality can vary with the configured provider, so tests assert
  ordering contracts rather than vendor output.

## Evidence

- `../../ecphory/ecphory.go` and `../../ecphory/ranker.go`
- `cmd/retrieve.go` and its tests
