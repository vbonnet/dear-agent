# ADR-012: Role-based model and provider routing

Status: Accepted (2026-05-03; verified 2026-07-17)

## Context

Workflow authors need stable intent such as `researcher`, while operators need
to change models, providers, and fallbacks without rewriting every workflow.
Literal model IDs alone couple durable work definitions to volatile inventory.

## Decision

`pkg/llm` separates four concerns:

- provider implementations execute requests;
- model-family resolution maps model IDs to providers;
- configuration maps roles to ordered model choices;
- the router applies fallback, circuit breaking, and policy.

A literal model is an explicit override. Otherwise an AI node names a role and
the configured router resolves it. The workflow engine depends on its small
executor interface, not on any provider SDK.

## Alternatives

Hard-coding Anthropic in the runner breaks cross-harness operation. Provider
selection inside workflow YAML makes fallback policy non-central. Combining
construction and routing would give a stateless factory runtime policy state.

## Consequences

Role configuration becomes an operational source of truth and must be tested
against supported model families. Routing remains replaceable without changing
workflow semantics. Tests under `pkg/llm/provider`, `pkg/llm/router`, and
`cmd/workflow-run` verify the boundary.
