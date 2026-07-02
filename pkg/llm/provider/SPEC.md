# LLM Provider Routing Specification

<!-- Last audited at: 2026-07-02 -->

## Purpose

`pkg/llm/provider` maps model identifiers to provider families and constructs
provider implementations. It is the executable bridge between AGM's model-family
registry and the concrete API provider used by Engram, workflow, and router
callers.

## EARS Requirements

**LLMP-01** When the resolver receives an OpenRouter-hosted supported family model identifier, the system shall route it to the `openrouter` provider while preserving the upstream model identifier.

**LLMP-02** When the factory receives the `openrouter` provider family with API-key authentication available, the system shall construct an `OpenRouterProvider`.

**LLMP-03** When the factory constructs an `OpenRouterProvider` with a requested model identifier, the system shall preserve that model as the provider default.

**LLMP-04** When OpenRouter provider capabilities are reported, the system shall include default model identifiers for GLM, DeepSeek, Nemotron, and Qwen model families.

**LLMP-05** When OpenRouter authentication is unavailable or unsupported, the system shall reject provider construction with an explicit error instead of falling back to another provider family.

## BDD Traceability

- Feature: `agm/test/bdd/features/model_family_parity.feature`
