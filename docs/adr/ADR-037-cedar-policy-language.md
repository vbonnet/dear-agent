# ADR-037: Cedar as the persona/policy-enforcement language

Status: Accepted (2026-07-27)

## Context

Stage 1 of the agent-persona-system research (`research/2026-07-21-agent-persona-system-research.md`,
engram-research PR #163) left open how the persona system's policy-enforcement
layer — tool allow/deny/ask, path/resource guards — should be authored and
cross-harness enforced. A companion research pass
(`research/2026-07-21-agent-persona-policy-language-research.md`) answered
that question directly, evaluating EARS/Gherkin, Rego/OPA (Valentin's own
named candidate), CEL, Cerbos, and Cedar against the actual problem shape:
runtime interception of a live tool call (principal acting via action on
resource in context), not spec-time or test-time verification.

EARS/Gherkin were ruled out outright — they operate one layer up
(requirements/test authoring) and never evaluate a live request. Among the
runtime candidates, Rego/OPA is the most mature and has the largest
ecosystem, but the field is visibly moving away from it for
authorization-shaped problems specifically (the "Rego tax" — see Kyverno's
move off Rego, Kubernetes' own move to CEL-based admission control). Cerbos
has the strongest built-in testing story but is service-first by design,
which fights an embeddable, in-process, per-harness interception model.

Cedar was not on the original candidate list but surfaced directly from
researching "policy engines for AI agent tool calls": AWS chose Cedar for
Bedrock AgentCore's agent-to-tool gateway (GA 2026-03-03) specifically
because policy evaluation must be deterministic and immune to the model's
own reasoning ("the LLM has no say in the policy decision"), and Microsoft's
Agent Governance Toolkit (April 2026) independently ships Cedar as a
first-class backend for the same problem, alongside Rego. Two unrelated
vendors converging on the same non-Rego choice for the same problem in the
same quarter is a stronger signal than either alone. Cedar also has an
actively-maintained native Go implementation (`cedar-policy/cedar-go`),
fitting dear-agent's existing Go codebase the same way OPA's Go core would.

The research also surfaced a correction to Stage 1's framing: no candidate
language — Cedar, Rego, or otherwise — actually does ahead-of-time
compilation of one canonical policy into each harness's own native
permission-config surface (e.g. a generated Claude Code `settings.json`
fragment or a Codex `config.toml` block). The pattern the field has
converged on instead is **one canonical policy language plus one common
interceptor/gateway wired into each harness's own hook point** — which also
matches what Stage 1 already found for Omnigent's Pi adapter (a runtime
`tool_call` hook, not a compiled Pi config file).

## Decision

- Adopt **Cedar** as the canonical policy-enforcement language for the
  persona/policy layer: tool allow/deny/ask and resource/path guards are
  authored once, in Cedar, as the single source of truth.
- Enforce via a **shared policy-engine + per-harness runtime interceptor**
  pattern — one Cedar evaluation service/library wired into each harness's
  own tool-dispatch hook point (e.g. this repo's own pretool hooks, Omnigent's
  Pi `tool_call` hook) — not per-harness codegen of native permission config.
  The policy decision is made outside the agent's own reasoning, the same
  "deterministic enforcement over agent judgment" principle already on
  dear-agent's anti-pattern watchlist.
- Encode the three runtime outcomes as two ordered Cedar authorization
  decisions because Cedar itself returns only Allow or Deny. First evaluate
  whether the principal may invoke the tool at all; Deny maps to **deny**.
  If invocation is allowed, evaluate whether the invocation may proceed
  without confirmation; Allow maps to **allow**, while Deny maps to **ask**.
  Cedar's normal forbid-overrides-permit rule resolves conflicts within each
  decision, and the interceptor applies the fixed precedence
  `deny > ask > allow`. The mapping is runtime glue, not a third Cedar effect
  or an additional policy source.
- Keep **authored decisions separate from evaluator health**. A Cedar response
  with evaluation diagnostics is `policy_unavailable`, not a policy-authored
  deny from either ordered decision. In an interactive harness the interceptor
  maps that state to an explicit confirmation/escalation path; if confirmation
  is unavailable, it fails closed with an evaluator-error diagnostic rather
  than reporting that policy denied the call.
- Publish policy bundles atomically only after they parse, validate against the
  Cedar schema, and pass the deterministic policy fixture suite. A rejected
  candidate never replaces the active bundle. The evaluator retains a durable
  last-known-good bundle across restart; reload readers observe either the
  complete old bundle or the complete new bundle, never a partial update.
- Rego/OPA remains the documented fallback if Cedar's younger ecosystem
  (smaller `cedar-go`, no turnkey `cedar test`-equivalent at research time)
proves insufficient in implementation.

## Implementation acceptance gates

Before a Cedar interceptor can become a blocking runtime path, executable
tests must prove all of the following:

1. A malformed or fixture-failing candidate bundle leaves the active
   last-known-good bundle and its version unchanged.
2. Concurrent evaluation during reload observes one whole validated bundle,
   never mixed schema and policy generations.
3. A Cedar diagnostic or evaluation exception returns
   `policy_unavailable`, not `deny`, and records the bundle version and
   diagnostic without request secrets.
4. Interactive `policy_unavailable` requests enter the harness confirmation
   path; non-interactive requests fail closed with a distinct evaluator error.
5. Restart restores the last-known-good bundle before the interceptor accepts
   tool calls.

The shared evaluator SPEC and per-harness interceptor BDD scenarios must carry
these cases; unit tests of Cedar Allow/Deny alone do not satisfy this gate.

## Alternatives

**Rego/OPA** — rejected as the primary choice despite being more mature and
having the larger third-party agent-guardrail ecosystem, because it is
general-purpose where Cedar is purpose-built for exactly this
principal/action/resource/context decision shape, and because the newest,
most on-point prior art in this exact domain (AWS, Microsoft) chose Cedar
over it when given the choice between the two.

**CEL** — rejected as the canonical authoring layer; it is an expression
primitive other systems (Kubernetes admission control, Cerbos) build a
policy format on top of, not a policy-authoring system on its own. Worth
reconsidering as the condition/expression language inside a bespoke format
if Cedar proves insufficient.

**Cerbos** — rejected due to its service-first deployment posture, which
mismatches an embeddable, in-process-per-harness interception model, despite
having the strongest out-of-the-box deterministic-testing story of the
group. Worth revisiting only if a centralized policy service becomes the
right shape instead of per-harness in-process interception.

**Per-harness native-config codegen** (compile canonical policy directly
into each harness's own settings format) — rejected; no researched
prior art actually does this, and the shared-interceptor pattern already
matches Stage 1's own finding for the Pi adapter.

## Consequences

The persona/policy layer gets one deterministic, formally-analyzable source
of truth instead of N harness-specific permission configs to keep in sync.
Implementation work is now scoped as: a Cedar policy schema for
tool/resource/context, a shared evaluation library, and one interceptor per
harness hook point (starting with dear-agent's own pretool hooks). Cedar's
thinner ecosystem and unproven testing tooling (relative to `opa test` or
Cerbos's YAML test suites) are accepted risks, bounded by atomic validation,
last-known-good recovery, and the executable liveness gates above. Rego/OPA is
the fallback if Cedar blocks implementation. The historical `0d1b3ed0` Rego
re-evaluation identifier could not be resolved in the canonical Beads store;
this ADR supersedes that unverified follow-up as the durable decision record.
