# Ecphory decisions

Status: Accepted

## Context

Ecphory must retrieve useful context with predictable local behavior while
allowing optional model-assisted ranking. Source collections can change between
queries, and remote ranking can be unavailable.

## Decisions

1. **Candidate pipeline.** Build deterministic candidates from the current
   searchable source, then rank the bounded set. Do not make a remote provider
   the source of record.
2. **Per-query index.** Construct the in-memory frontmatter index for the
   current search input. This favors correctness and isolation over a hidden
   long-lived cache.
3. **Local fallback.** A local ranker remains available when no configured
   model provider succeeds.
4. **Bounded provider use.** Provider calls use explicit time and rate limits;
   a provider failure returns to the local path rather than failing retrieval.

## Consequences

- Retrieval works offline and does not depend on one vendor.
- Repeated searches pay index construction cost.
- Provider-specific ranking is optional and may change result order.

## Evidence

- `ecphory.go`, `index.go`, `ranker.go`, and their tests
- `ranking/`
