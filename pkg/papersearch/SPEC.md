# Paper Search Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/papersearch` searches academic paper APIs through arXiv, Semantic Scholar,
and a multi-backend fan-out searcher. It normalizes results into one `Paper`
shape while preserving rate-limit, error, and deduplication behavior per
backend.

## Requirements

**PAPERSEARCH-01** When an arXiv client is created without an HTTP client, the system shall use `http.DefaultClient`.

**PAPERSEARCH-02** When an arXiv search has no keywords, the system shall return no papers and no error.

**PAPERSEARCH-03** When an arXiv search runs, the system shall wait on the configured rate limiter before issuing the HTTP request.

**PAPERSEARCH-04** When parsing arXiv Atom results, the system shall trim title and summary text and derive the paper ID from the `/abs/` URL suffix.

**PAPERSEARCH-05** When a Semantic Scholar client is created, the system shall read `SEMANTIC_SCHOLAR_API_KEY` into the optional API key field.

**PAPERSEARCH-06** When a Semantic Scholar search has an empty query, the system shall return no papers and no error.

**PAPERSEARCH-07** When a Semantic Scholar search has an API key, the system shall send it in the `x-api-key` header.

**PAPERSEARCH-08** When Semantic Scholar returns a paper without an abstract, the system shall fall back to the title as abstract text.

**PAPERSEARCH-09** When a multi-searcher is configured with arXiv and Semantic Scholar backends, the system shall query both backends concurrently.

**PAPERSEARCH-10** When a paper search backend errors or panics, the system shall log the backend failure and continue merging other backend results.

**PAPERSEARCH-11** When multi-search results have duplicate titles after trim and lowercase normalization, the system shall keep only the first backend-ordered result.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
