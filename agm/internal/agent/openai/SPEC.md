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

**OAI-05** When adding one or more messages to an OpenAI session, the system shall synchronize in-process access, reload current JSONL history, atomically replace that history with the appended messages, persist matching metadata, and restore the prior history if metadata persistence fails.

**OAI-06** When an OpenAI adapter creates a session, the system shall persist its resolved model and non-secret runtime settings; when another process reconstructs that session's adapter, it shall restore the persisted model, temperature, token limit, base URL, Azure mode, and Azure API version while obtaining the API credential only from current runtime configuration.

**OAI-07** When an OpenAI adapter sends to a session, the system shall serialize the complete history-read, provider-completion, and persistence transaction across adapter instances and processes; it shall send the latest completed history plus the new user message, commit the user and assistant messages as one completed turn only after provider success, and leave durable history unchanged when completion fails.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/model_family_parity.feature`
