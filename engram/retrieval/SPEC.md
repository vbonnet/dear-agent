# Engram Retrieval Service Specification

<!-- Last audited at: 2026-08-29 -->

## Purpose and Ownership

`github.com/vbonnet/dear-agent/engram/retrieval` is the sole package owner for
high-level Engram search. It provides a public facade over ecphory indexing and
ranking, Engram parsing, path resolution, result limiting, and best-effort
access tracking.

In-repository callers include the Engram `retrieve` and `tokens estimate`
commands and the AGM Engram library client. External Go callers may import the
same package. No duplicate internal facade or compatibility proxy is part of
this contract.

## Public Interface

- `NewService()` constructs a retrieval service with a parser and access
  tracker.
- `Service.Search(ctx, opts)` resolves a knowledge directory, selects and
  optionally ranks candidates, parses results, and records access metadata.
- `Service.Close()` flushes pending tracking updates on a best-effort basis.
- `SearchOptions` carries the path, query, filters, result limit, and
  API-ranking choice. Its session and transcript fields are retained for caller
  compatibility but are not consumed by the current service.
- `SearchResult` carries the path, parsed Engram, and any API ranking metadata.

Callers own the service lifecycle and should call `Close` after their final
search.

## EARS Requirements

**ERT-01** When search receives an absolute Engram path that does not exist, the system shall return an error before building an index.

**ERT-02** When search receives an empty or relative Engram path, the system shall resolve the default `~/.engram/core/engrams` directory before falling back to a path relative to the current working directory.

**ERT-03** When the resolved Engram path exists, the system shall build an ecphory index from that path before applying result filters.

**ERT-04** When tag filters are supplied, the system shall select candidates by tag before type filtering or unfiltered listing.

**ERT-05** When no tag filters are supplied and a type filter is supplied, the system shall select candidates by Engram type.

**ERT-06** When no tag or type filters are supplied, the system shall consider every indexed Engram candidate.

**ERT-07** When API ranking is requested without `ANTHROPIC_API_KEY`, the system shall fall back to locally selected results instead of failing the search.

**ERT-08** When API ranking is requested with API credentials, the system shall return paths in ranker order and attach relevance score and reasoning to matching results.

**ERT-09** When a positive limit is smaller than the candidate count, the system shall return no more than that number of result paths.

**ERT-10** When a candidate file cannot be parsed as an Engram, the system shall skip that candidate and continue returning parseable results.

**ERT-11** When parsed results are returned, the system shall record best-effort access metadata for each returned Engram path.

**ERT-12** When the retrieval service is closed, the system shall flush pending tracking updates and shall not fail the caller if the flush itself fails.

## Behavioral Invariants

- Tags take precedence when both tag and type filters are supplied.
- Missing API credentials are an expected local-only mode. Ranker construction
  and ranking errors after credentials are configured are returned to the
  caller with context.
- Unparseable candidate files do not fail the whole search.
- Tracking is buffered by the service; `Search` alone does not guarantee that
  metadata has reached disk.
- Access tracking currently records only the returned path and access time; it
  does not consume `SessionID` or `Transcript`.
- The service rebuilds its index for each search and does not promise caching,
  pagination, custom ranker injection, or concurrent-use safety.

## BDD and Test Traceability

- SPEC ownership: `agm/test/bdd/features/engram_knowledge_guardrails.feature`
- Strict EARS inventory: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`
- Package tests: `engram/retrieval/retrieval_test.go`
- Integration tests: `engram/retrieval/retrieval_integration_test.go`
- Command regression: `engram/cmd/engram/cmd/tokens_estimate_integration_test.go`

See `ARCHITECTURE.md` for the ownership seam and dependency flow. Durable
behavior decisions are recorded under `docs/adrs/`.
