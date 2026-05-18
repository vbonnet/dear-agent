# ADR-012: Provider Transport Layer (Roles → Providers Routing)

**Status**: Proposed
**Date**: 2026-05-03
**Context**: dear-agent's "bring whatever models you prefer" philosophy

## Context

dear-agent's stated philosophy is methodology-first: users bring whatever
models, tools, and harnesses they prefer. The repo already has substantial
infrastructure toward that goal in `pkg/llm/provider/`:

- A `Provider` interface (`Name`, `Generate`, `Capabilities`) with concrete
  implementations for Anthropic, Ollama, OpenRouter, and Vertex AI (Claude
  + Gemini).
- A `Factory` that auto-detects auth (`pkg/llm/auth`) and constructs a
  provider for a given family.
- Circuit-breaker + rate-limiter wrappers (`circuitbreaker.go`,
  `ratelimiter.go`).
- A `costtrack` package that records per-call cost via `CostSink` and a
  `BudgetConfig` for per-project / per-model spend caps.

But three pieces are missing if a workflow author wants to say "use the
research role here" and have the engine pick a model, a provider, and a
fallback chain on its own:

1. **A native OpenAI provider.** The repo has Azure-OpenAI plumbing in
   `agm/internal/llm/openai_client.go` but no `pkg/llm/provider`-shaped
   OpenAI implementation. OpenRouter can proxy OpenAI but it isn't direct.
2. **A model-id → family resolver.** The factory takes a *family* string
   ("anthropic", "gemini", …) but workflow nodes carry a *model id*
   ("claude-opus-4-7", "gpt-5-pro"). Today the workflow runner hard-wires
   `AnthropicProvider` regardless (`cmd/workflow-run/main.go:84-89`), which
   means anything other than Claude silently routes through Anthropic's
   API and fails.
3. **A role-aware router with primary/secondary/tertiary fallback.** When
   Anthropic is rate-limited or down, a request that asked for the
   "implementer" role should fall through to OpenAI, then to Gemini, with
   a circuit breaker per-model so we don't keep retrying a dead path.
   `pkg/llm/provider/circuitbreaker.go` exists but is per-provider and
   single-fallback; there is no router.

The deep-research tool already implements something close to a multi-
provider chain (`research/cmd/deep-research/research/factory.go:118-159`)
but it's tool-specific, parallel-aggregation-shaped, and not reusable from
the workflow engine.

The user-stated goal in this task brief: a `ProviderTransport` interface
plus a `ProviderResolver` that maps roles → (transport, model, credentials,
fallbacks).

## Decision

Add three small layers on top of the existing `pkg/llm/provider` package
rather than rewriting it:

1. **`pkg/llm/provider/openai.go`** — a native `OpenAIProvider` that
   implements `provider.Provider`, using `github.com/sashabaranov/go-openai`
   (already a transitive dependency through `agm/internal/llm`).

2. **`pkg/llm/provider/resolver.go`** — `Resolver` resolves a model id to
   a provider family and (optionally) a normalized model name. It supports:
   - Bare ids: `"claude-opus-4-7"` → `("anthropic", "claude-opus-4-7")`.
   - Prefixed ids: `"openai/gpt-5-pro"`, `"ollama:llama3.2"` →
     explicit family override.
   - A small built-in mapping table (extensible) so adding `"gemini-3.1-pro"`
     does not require code changes — it matches the `gemini-` prefix and
     routes to the `gemini` family.

3. **`pkg/llm/router/`** — `Router` (the user's "ModelRouter") owns role
   → (primary, secondary, tertiary) mappings, the per-model circuit
   breakers, and the fallback loop. It exposes a single `Generate(ctx,
   role, *GenerateRequest) (*GenerateResponse, error)` entry point and
   surfaces a thin `AIExecutor` adapter so the workflow engine can swap
   in the router without touching the runner.

   Roles are loaded from `config/roles.yaml` (or an explicit path passed
   in by callers). The schema mirrors what the task brief specified:

   ```yaml
   version: 1
   roles:
     research:
       primary: gemini-3.1-pro
       secondary: claude-opus-4-7
       tertiary: gpt-5.5-pro
     implementer:
       primary: claude-opus-4-7
       secondary: gpt-5.5-pro
       tertiary: gemini-3.1-pro
     orchestrator:
       primary: claude-sonnet-4-6
       secondary: gemini-2.5-flash
       tertiary: gpt-5.4-mini
   default_role: orchestrator
   ```

4. **Workflow engine integration.** Add an optional `Role` field to
   `pkg/workflow.AINode`. The new `router.AIExecutor` adapter reads
   `node.Model` first; if empty it falls back to `node.Role`; if that's
   also empty it uses the configured default role. The existing
   `cmd/workflow-run/main.go` switches its default `AIExecutor` from a
   hardcoded `AnthropicProvider` to a router-backed adapter, with a
   `-roles` flag for an explicit config path.

5. **Budget + credentials.** Per-call cost tracking already happens inside
   each provider via `costtrack.CostSink`. The router does not duplicate
   that — it sets the `Component` field of the cost record to the role
   name (e.g. `"role:research"`) so a future budget gate can spend-cap by
   role. Credentials continue to come from `pkg/llm/auth` (env vars,
   already supported); a future ADR can add keychain support without
   touching the router.

## Rationale

### Why a separate `router` package and not a method on `Factory`?

The factory's job is "make me a provider for family X with auth Y." That's
a stateless, deterministic operation. The router is *stateful* (per-model
circuit breakers, role config, last-known-good cache) and depends on the
factory plus the resolver. Mixing them couples the construction surface
to the routing policy and makes the factory hard to use in the cases where
you *want* a specific provider (tests, single-model tools).

### Why add a `Role` field to `AINode` instead of inferring from `Model`?

The workflow author's intent is different in the two cases:

- `model: claude-opus-4-7` says "I specifically want this model. Fail if
  it can't run."
- `role: implementer` says "I want whatever the operator's policy says
  the implementer should be today." The operator can rebalance the role
  spec without touching every workflow file.

Keeping both, with `Model` taking precedence, lets workflows be either
prescriptive or policy-driven on a per-node basis.

### Why per-model circuit breakers, not per-provider?

A single Anthropic outage trips one breaker that affects every Anthropic
model. But operators frequently mix providers within a family (e.g.
`claude-opus-4-7` for orchestration, `claude-sonnet-4-6` for cheap
batch work) and want them to fail independently — sonnet being throttled
shouldn't trip opus. Per-model is the granularity that matches the
fallback-chain shape, where each rung is identified by a model id.

### Why not introduce a brand-new `ProviderTransport` interface?

The task brief proposed a new `ProviderTransport.Send(ctx, messages,
config)` interface, but `pkg/llm/provider.Provider` already has the
needed shape (`Generate(ctx, *GenerateRequest)`) and four working
implementations. Adding a parallel interface would force every existing
provider to be ported and would split the code base. Instead, the router
consumes the existing `Provider` interface, and any new transport is just
another `Provider` implementation.

## Alternatives Considered

1. **Use OpenRouter as the universal transport.** Pro: one HTTP path,
   one auth surface. Con: extra hop, extra cost, vendor lock-in to
   OpenRouter's pricing and uptime, and we lose direct access to
   provider-specific features (Anthropic prompt caching, OpenAI structured
   output, Vertex AI ADC). Rejected.

2. **Move the routing logic into the workflow runner.** Pro: no new
   package. Con: couples the workflow engine to model selection, makes
   the runner harder to test, and prevents reuse from non-workflow
   callers (the `engram` and `wayfinder` tools also need routed model
   access). Rejected.

3. **Encode roles directly in `AINode.Model` (e.g., `model: "@research"`).**
   Pro: no schema change to `AINode`. Con: overloads a single field with
   two meanings and makes "use this exact model" syntactically awkward.
   Rejected in favor of a separate `Role` field.

## Consequences

### Positive

- Workflow authors can declare *intent* (`role: research`) and let
  operators tune *policy* (which model is "research" today) without
  re-shipping workflow files.
- Adding a new provider is one file in `pkg/llm/provider/` plus one
  resolver-table entry. The router needs no changes.
- Per-model circuit breakers stop a single outage from cascading across
  unrelated models in the same family.
- Existing direct callers of `pkg/llm/provider.Factory` are untouched.

### Negative / costs

- One more concept ("role") for workflow authors to learn. Mitigated by
  keeping `Model` as the literal escape hatch.
- `roles.yaml` becomes another config file the operator must maintain.
  Mitigated by shipping a default `config/roles.yaml` checked into the
  repo and by accepting empty/missing config (the router falls back to
  `node.Model` only).

### Follow-ups (not in scope here)

- A budget-gate wrapper that calls `costtrack.CheckBudget` *before*
  routing and short-circuits when the role's spend cap is exceeded.
- Keychain credential storage (`pkg/llm/auth`).
- Streaming responses (the existing `Provider` interface returns a single
  `*GenerateResponse`; streaming would need a parallel method).
- Per-role rate limiters (today rate-limit is per-provider).

## Implementation Notes

The new code is concentrated in:

- `pkg/llm/provider/openai.go` (+ `_test.go`) — native OpenAI provider.
- `pkg/llm/provider/resolver.go` (+ `_test.go`) — model-id → family.
- `pkg/llm/router/router.go` (+ `_test.go`) — role-based fallback.
- `pkg/llm/router/config.go` (+ `_test.go`) — `roles.yaml` loader.
- `config/roles.yaml` — default role mapping checked into the repo.
- `pkg/workflow/types.go` — adds `Role string` to `AINode`.
- `cmd/workflow-run/main.go` — wires the router as the default executor.

No changes to the existing `Provider` interface, the factory, the circuit
breaker, the rate limiter, or any existing provider implementation.
