# OpenAI Agent Adapter Specification

<!-- Last audited at: 2026-07-21 -->

## Purpose

`agm/internal/agent/openai` provides the legacy OpenAI API-backed agent client
and JSONL session store used by AGM compatibility paths. It validates model and
credential configuration, preserves conversation history, and reports typed
errors for callers that need harness-neutral failure handling.

## EARS Requirements

**OAI-01** When constructing an OpenAI client, the system shall require an API key and validate the configured model before creating the underlying API client.

**OAI-02** When no model is configured, the system shall use `OPENAI_MODEL` and then the package default model as fallbacks.

**OAI-03** When a model does not support streaming, the system shall report that through the model capability helper.

**OAI-04** When creating an OpenAI session, the system shall create a private session directory, persist metadata, and reject duplicate session IDs.

**OAI-05** When adding a message to an OpenAI session, the system shall synchronize access to prevent data races, update in-memory metadata, append the JSONL message, and persist the updated metadata.

**OAI-06** When an OpenAI adapter creates a session, the system shall persist its resolved model and non-secret runtime settings; when another process reconstructs that session's adapter, it shall restore the persisted model, temperature, token limit, base URL, Azure mode, and Azure API version while obtaining the API credential only from current runtime configuration.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/model_family_parity.feature`
