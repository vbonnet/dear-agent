# ADR-004: Use a tiered Engram retrieval pipeline

Status: Accepted

## Context

Engram queries range from exact known-memory lookups to semantic recall. Sending
every query to an external model adds latency, cost, and availability risk,
while local lexical matching alone misses useful context.

## Decision

Retrieval progresses from deterministic local candidates to an optional
configured model reranker. With `--no-api`, or when Anthropic credentials are
absent, the command returns index candidates locally. Once API ranking is
selected and credentials are present, provider construction and ranking errors
are returned to the caller rather than silently downgraded.

The command surface exposes the result and available explanation data; it does
not promise a particular external provider.

## Consequences

- Explicit no-API queries and queries without credentials remain local.
- Higher-cost ranking is explicit and optional.
- Ranking quality can vary with the configured provider, so tests assert
  ordering contracts rather than vendor output.

## Evidence

- `../../ecphory/ecphory.go` and `../../ecphory/ranker.go`
- `cmd/retrieve.go` and its tests
