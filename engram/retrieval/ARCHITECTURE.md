# Engram Retrieval Architecture

<!-- Ownership and dependency paths reconciled at: 2026-08-29 -->

## Context

Engram needs one high-level search interface that can be used by its own CLI,
AGM, and external Go programs. The repository previously carried byte-near
duplicate public and internal implementations. That split made contract, tests,
workflow coverage, and caller migrations drift independently.

The canonical owner is now the public package:

```text
github.com/vbonnet/dear-agent/engram/retrieval
```

Go's `internal` visibility rules make the public location necessary for AGM and
external callers. The retired internal path has no forwarding wrapper: a proxy
would preserve two apparent owners without adding abstraction.

## Ownership and Dependency Flow

```text
Engram retrieve command ---------+
Engram tokens estimate command --+--> engram/retrieval
AGM Engram library client --------+          |
external Go callers --------------+          +--> engram/ecphory
                                              +--> pkg/engram parser
                                              +--> internal/tracking
```

`engram/retrieval` owns orchestration and the public types. Ecphory owns index
construction, candidate filtering primitives, and optional semantic ranking.
The parser owns Engram decoding. Tracking owns buffered metadata updates.

No consumer should import or recreate a second retrieval facade. Changes to
path resolution, filtering order, ranking fallback, result mapping, or access
tracking belong in this package and its tests.

## Search Flow

1. `NewService` creates a parser and buffered tracker.
2. `Search` resolves the requested Engram directory.
3. A fresh ecphory index is built for that directory.
4. Tags, type, or an unfiltered listing selects candidates; tags take
   precedence when both filters are supplied.
5. If API ranking is disabled, no semantic reordering is applied. If ranking is
   enabled but `ANTHROPIC_API_KEY` is absent, the same local path is used; the
   facade does not promise stable ordering for locally selected candidates.
6. With credentials configured, ecphory ranks candidates. Construction or
   ranking failures are returned, while successful ranking contributes score
   and reasoning metadata.
7. The positive result limit is applied, parseable Engrams become public
   `SearchResult` values, and access is recorded for each returned path.
8. `Close` attempts to flush buffered tracking updates. Flush errors are logged
   and deliberately do not fail callers.

## Invariants

- A nonexistent absolute path fails before index construction.
- Empty and relative paths try the default Engram directory before the current
  working directory.
- Tag selection precedes type selection.
- Only missing credentials trigger silent API-to-local fallback.
- A malformed candidate is skipped without discarding other parseable results.
- Access tracking occurs only for parsed results and is not durable until the
  service is closed or the tracker otherwise flushes.
- The public interface stays small while coordinating substantial retrieval
  behavior without duplicating dependency-owned indexing, parsing, ranking, or
  persistence logic.

## Verification Surfaces

- `retrieval_test.go` covers construction, path resolution, filter precedence,
  and limiting.
- `retrieval_integration_test.go` covers real index, parser, and filter
  interaction with local ranking disabled.
- `engram/cmd/engram/cmd/tokens_estimate_integration_test.go` proves the CLI's
  retrieval mode crosses the public package interface.
- `agm/test/bdd/features/engram_knowledge_guardrails.feature` enforces the
  canonical SPEC ownership.
- `.github/workflows/cross-platform-test.yml` exercises the package on Windows.

## Durable Decisions

- [ADR-001: Per-search index building](docs/adrs/ADR-001-per-search-index-building.md)
- [ADR-002: Tracking integration in the service layer](docs/adrs/ADR-002-tracking-integration-in-service-layer.md)
- [ADR-003: API ranking fallback strategy](docs/adrs/ADR-003-api-ranking-fallback-strategy.md)

The ownership consolidation is documented here because it removes a duplicate
module seam; it does not introduce a new durable behavior decision.
