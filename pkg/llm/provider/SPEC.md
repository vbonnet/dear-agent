# LLM Provider Routing Specification

<!-- Last audited at: 2026-07-21 -->

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

**LLMP-06** When OpenRouter capabilities advertise Nemotron or Qwen defaults, the system shall publish provider-canonical model identifiers that correspond to routable model pages.

**LLMP-07** When a circuit-breaker primary or fallback provider returns a nil response without an error, the system shall convert it to an explicit provider error rather than returning a successful nil response.

**LLMP-08** When a circuit breaker returns a response, the system shall expose the provider, model, and fallback state that actually produced it.

## BDD Traceability

- Feature: `agm/test/bdd/features/model_family_parity.feature`
