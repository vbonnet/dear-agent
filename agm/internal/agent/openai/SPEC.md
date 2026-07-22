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

**OAI-05** When adding one or more messages to an OpenAI session, the system shall synchronize cross-process access, reload current JSONL history and on-disk metadata, atomically replace that history with the appended messages, persist matching counts without overwriting newer metadata fields, and restore the prior history if metadata persistence fails.

**OAI-06** When an OpenAI adapter creates a session, the system shall persist its resolved model and non-secret runtime settings; when another process reconstructs that session's adapter, it shall restore the persisted model, temperature, token limit, base URL, Azure mode, and Azure API version while obtaining the API credential only from current runtime configuration.

**OAI-07** When an OpenAI adapter sends to a session, the system shall serialize the complete history-read, provider-completion, and persistence transaction across adapter instances and processes; it shall send the latest completed history plus the new user message, commit the user and assistant messages as one completed turn only after provider success without overwriting newer session metadata, and leave durable history unchanged when completion fails.

**OAI-08** When an OpenAI adapter waits for the store-scoped session lock or calls the provider, the system shall honor the caller's context and apply a finite provider deadline even for legacy callers without one; cancellation or timeout shall release the lock and shall leave durable history unchanged.

**OAI-09** When an OpenAI adapter clears session history, the system shall serialize the mutation, reload the current on-disk metadata under that boundary, atomically replace only the message history, and preserve the session model, title, working directory, and persisted non-secret runtime configuration so a later process reconstructs identical delivery settings without losing updates from another process.

**OAI-10** When any OpenAI session manager updates title, working directory, or persisted runtime configuration, the system shall acquire the same store-scoped session lock as message commit and history clear, reload current metadata under that lock, apply only the requested field change, and preserve independent updates made by another process.

**OAI-11** When the pure OpenAI API adapter reports session status, the system shall report an existing stored session as active and a missing stored session as terminated without invoking tmux; tmux-backed Codex CLI readiness belongs exclusively to the `codex-cli` adapter.

**OAI-12** When documenting the legacy OpenAI API adapter, the system shall distinguish direct Go adapter construction from the supported AGM control plane, shall limit production delivery to already-registered `openai` or `gpt` manifests, and shall not advertise public CLI creation or resume commands that harness validation rejects.

**OAI-13** When an OpenAI session is deleted while another adapter may deliver to the same store and session ID, deletion shall acquire the store-scoped session lock shared with provider completion, and delivery shall revalidate authoritative on-disk metadata under that lock before provider work; either a started completed turn shall commit before deletion or a completed deletion shall reject the send without calling the provider.

**OAI-14** When an OpenAI adapter is reconstructed or its readiness is checked for a request-scoped delivery, the system shall use the caller context while waiting for the authoritative store lock, shall return cancellation or deadline errors without reporting the session terminated, and shall not retain the surrounding lifecycle lock after the request is canceled.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/model_family_parity.feature`
