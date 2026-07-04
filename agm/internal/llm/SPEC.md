# AGM LLM Client Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/llm` contains legacy AGM LLM clients for semantic search and
OpenAI-compatible chat completions. New model-family routing lives in
`pkg/llm/provider`, but this package still needs a local contract because it
touches provider configuration, model defaults, rate limiting, response parsing,
and search-result caching used by AGM-facing workflows.

## EARS Requirements

**AGM-LLM-01** When a Vertex AI search client is created without a location, the system shall default the location to `us-central1`.

**AGM-LLM-02** When a Vertex AI search client is created without a model ID, the system shall default the model ID to `claude-3-5-haiku@20241022`.

**AGM-LLM-03** When a Vertex AI search client is created without a project ID, the system shall return an actionable configuration error.

**AGM-LLM-04** When semantic search runs, the system shall enforce the configured rate limit before creating the Vertex AI prediction request.

**AGM-LLM-05** When semantic search builds a prompt, the system shall include the query and each candidate session's ID, name, tags, and project when present.

**AGM-LLM-06** When semantic search receives no predictions, the system shall return an empty result list.

**AGM-LLM-07** When semantic search receives malformed provider output, the system shall return a parse error instead of fabricating matches.

**AGM-LLM-08** When an OpenAI-compatible client is created without an explicit provider, the system shall auto-detect Azure OpenAI from Azure environment variables and otherwise use standard OpenAI.

**AGM-LLM-09** When an OpenAI-compatible client is configured for Azure, the system shall require API key, endpoint, deployment, and API version before creating the client.

**AGM-LLM-10** When search results are cached, the system shall return cached results only until their TTL expires and shall allow expired entries to be removed.

## BDD Traceability

- `agm/test/bdd/features/model_family_parity.feature`

## Package Test Traceability

- `agm/internal/llm/client_test.go`
- `agm/internal/llm/openai_client_test.go`
- `agm/internal/llm/cache_test.go`
- `agm/internal/llm/example_test.go`
