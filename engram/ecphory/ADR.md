# Ecphory decisions

Status: Accepted

## Context

Ecphory retrieves context through a constructor-scoped index and a configured
model ranker. Provider calls may fail after construction, and source
collections may change while an instance remains alive.

## Decisions

1. **Candidate pipeline.** Build deterministic candidates from the indexed
   source, then rank the bounded set. The provider is not the source of record.
2. **Instance-scoped index.** `NewEcphory` builds the frontmatter index once;
   callers create a new instance to observe later source changes.
3. **Required configured ranker.** Construction fails unless Vertex AI or a
   valid Anthropic credential can initialize the ranker.
4. **Unranked fallback after construction.** If a later provider call fails,
   `Query` returns the existing candidates in index order rather than invoking
   a separate local ranker.

## Consequences

- Repeated queries reuse one index and may not observe source changes.
- Missing provider configuration prevents construction.
- Provider-call failures can reduce ordering quality without losing candidates.

## Evidence

- `ecphory.go`, `index.go`, `ranker.go`, and their tests
- `ranking/`
