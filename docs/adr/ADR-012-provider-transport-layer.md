# ADR-012: Provider Transport — Roles → Providers Routing

**Status**: Proposed (2026-05-03)

`pkg/llm/provider/` already has a `Provider` interface and four
implementations (Anthropic, Ollama, OpenRouter, Vertex AI), per-provider
circuit breakers, rate limiters, and a cost tracker. Three pieces are
missing if a workflow author wants to say "use the research role here"
and have the engine pick model + provider + fallback chain:

1. A **native OpenAI provider**. We proxy through OpenRouter today.
2. A **model-id → family resolver.** The factory takes a family
   ("anthropic", "gemini"); workflow nodes carry a model id
   ("claude-opus-4-7"). The current runner hard-wires `AnthropicProvider`
   regardless, so anything else silently routes to Anthropic and fails.
3. A **role-aware router** with primary/secondary/tertiary fallback and
   per-model circuit breakers.

Add three small layers on top of the existing package; don't rewrite it.

- **`pkg/llm/provider/openai.go`** — native `OpenAIProvider` over
  `github.com/sashabaranov/go-openai` (already a transitive dep through
  `agm/internal/llm`).
- **`pkg/llm/provider/resolver.go`** — bare ids
  (`claude-opus-4-7` → `anthropic`), prefixed overrides
  (`openai/gpt-5-pro`, `ollama:llama3.2`), and an extensible prefix
  table so a new family name is data, not code.
- **`pkg/llm/router/`** — `Router` owns role mappings (`config/roles.yaml`),
  per-model breakers, the fallback loop, and a `Generate(ctx, role,
  *GenerateRequest)` entry point. Exposes an `AIExecutor` adapter so the
  workflow runner swaps in without changes.

`AINode` gains an optional `Role`. `Model` takes precedence; absent both,
the configured `default_role` is used. The trade-off is two ways to
declare intent: literal `model:` ("I want this specifically — fail if
it can't run") and policy-driven `role:` ("whatever the operator's
spec says today, recomputable without re-shipping workflows"). We
accepted both — prescriptive workflows stay readable, operators rebalance
roles without editing every YAML.

### Why this shape

- **Why a separate `router` package, not a factory method?** The factory
  is stateless construction; the router is *stateful* (breakers, role
  config, last-known-good cache) and depends on the factory + resolver.
  Mixing them couples construction surface to routing policy.
- **Why per-model breakers, not per-provider?** Operators mix providers
  within a family (Opus for orchestration, Sonnet for cheap batch).
  Sonnet being throttled should not trip Opus.
- **Why not introduce a brand-new `ProviderTransport` interface?**
  `Provider.Generate` already has the needed shape; adding a parallel
  interface would force every existing provider to be ported and split
  the codebase.

### Alternatives rejected

- **OpenRouter as the universal transport.** Extra hop and cost, vendor
  lock-in, and we lose provider-specific features (Anthropic prompt
  caching, OpenAI structured output, Vertex ADC).
- **Routing logic in the workflow runner.** Couples the engine to model
  selection; prevents reuse from non-workflow callers (`engram`,
  `wayfinder`).
- **Encode roles in `AINode.Model` (`model: "@research"`).** Overloads
  one field with two meanings; awkward for the literal case.

### Out of scope (follow-ups)

- A budget-gate wrapper calling `costtrack.CheckBudget` before routing.
- Keychain credential storage in `pkg/llm/auth`.
- Streaming responses (parallel method on `Provider`).
- Per-role rate limiters (today rate-limit is per-provider).

The router stamps `costtrack.Component` with the role name (e.g.
`"role:research"`) so a future budget gate can spend-cap by role.
Credentials continue to come from `pkg/llm/auth`. New code lives in
`pkg/llm/provider/{openai,resolver}.go` and `pkg/llm/router/`; no changes
to the existing `Provider` interface, factory, breakers, or rate limiter.
