# Engram Retrieval Service Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/internal/retrieval` is the high-level search boundary used by Engram
CLI, API, and plugin surfaces. It wraps the ecphory index, applies lightweight
tag/type filtering, optionally delegates ranking to the Anthropic-backed
ecphory ranker, and records best-effort access metadata for returned engrams.

The package is intentionally harness-neutral. It searches the configured Engram
knowledge directory and returns parsed Engram records without assuming a Claude,
Codex, Gemini, AGY, or OpenCode caller.

## EARS Requirements

**ERT-01** When search receives an absolute Engram path that does not exist, the system shall return an error before building an index.

**ERT-02** When search receives an empty or relative Engram path, the system shall resolve the default `~/.engram/core/engrams` directory before falling back to a path relative to the current working directory.

**ERT-03** When the resolved Engram path exists, the system shall build an ecphory index from that path before applying result filters.

**ERT-04** When tag filters are supplied, the system shall select candidates by tag before type filtering or unfiltered listing.

**ERT-05** When no tag filters are supplied and a type filter is supplied, the system shall select candidates by Engram type.

**ERT-06** When no tag or type filters are supplied, the system shall consider every indexed Engram candidate.

**ERT-07** When API ranking is requested without `ANTHROPIC_API_KEY`, the system shall fall back to index-order results instead of failing the search.

**ERT-08** When API ranking is requested with API credentials, the system shall return paths in ranker order and attach relevance score and reasoning to matching results.

**ERT-09** When a positive limit is smaller than the candidate count, the system shall return no more than that number of result paths.

**ERT-10** When a candidate file cannot be parsed as an Engram, the system shall skip that candidate and continue returning parseable results.

**ERT-11** When parsed results are returned, the system shall record best-effort access metadata for each returned Engram path.

**ERT-12** When the retrieval service is closed, the system shall flush pending tracking updates and shall not fail the caller if the flush itself fails.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_knowledge_guardrails.feature`
- Package tests: `engram/internal/retrieval/retrieval_test.go`
- Package tests: `engram/internal/retrieval/retrieval_integration_test.go`

