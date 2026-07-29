# ADR-037: Cedar as the persona/policy-enforcement language

Status: Proposed (2026-07-27; implementation and migration pending)

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
- Keep **authored decisions separate from evaluator health** without
  weakening default deny or hard forbids. The interceptor advances past the
  invocation gate only when the response proves a validated applicable
  `permit`, returns Allow, **and carries no policy-evaluation diagnostics**.
  An Allow whose diagnostics report any errored policy is **deny**: a `forbid`
  that failed to evaluate — for example because the request entity lacked an
  attribute it tested — is neither proven applicable nor proven inapplicable,
  so treating that response as authorization would let an authored hard deny
  fail open. A proven applicable `forbid` remains **deny**
  even when another policy emits diagnostics; a Deny with no applicable
  permit also remains **deny**, regardless of diagnostics. Only after positive
  invocation authorization may diagnostics that prevent a trustworthy
  confirmation-free mapping produce `policy_unavailable`. In an interactive
  harness the interceptor maps that state to an explicit
  confirmation/escalation path; if confirmation is unavailable, it fails
  closed with an evaluator-error diagnostic rather than reporting that policy
  denied the call. A diagnostic can therefore never turn an authored
  invocation forbid or the absence of an invocation permit into
  user-overridable confirmation.
- Publish policy bundles atomically only after they parse, validate against the
  Cedar schema, and pass the deterministic policy fixture suite. Publication
  is a privileged control-plane operation, not an agent tool: the publisher
  authenticates a signed bundle envelope against a configured operator trust
  root, records its signer and digest, and is the only process identity allowed
  to replace the active bundle. The publisher's candidate staging input, the
  trust root, and the durable active-bundle directory live outside
  agent-writable workspaces and are not exposed through any harness file-write
  or reload capability. A
  syntactically valid bundle from an untrusted writer is rejected before
  validation and can never become active. A rejected candidate never replaces
  the active bundle. The evaluator retains a durable last-known-good bundle
  across restart; reload readers observe either the complete old bundle or the
  complete new bundle, never a partial update.
- Acquire one immutable bundle snapshot and version for each intercepted tool
  request before the invocation decision. Reuse that exact snapshot for the
  confirmation-free decision, even if a validated reload publishes a newer
  bundle between the two gates. A request never composes authorization results
  from different policy generations. An `ask` result is not an execution
  authorization: after confirmation, the dispatcher reacquires the current
  active snapshot at its final dispatch boundary. If its version differs, the
  prior confirmation is invalidated and both ordered decisions are rerun
  against the new snapshot; `deny` stops, `allow` proceeds, and `ask` requires
  a new confirmation bound to that version. The active generation is checked
  atomically when the dispatcher consumes that authorization; a concurrent
  reload restarts the loop instead of permitting a stale confirmed request.
- Project every path-bearing resource into one canonical filesystem identity
  before Cedar evaluation. The shared projector expands supported home forms,
  anchors relative paths to the interceptor's verified working directory,
  cleans traversal components, makes the path absolute, and resolves symlinks.
  For a missing leaf it resolves the deepest existing ancestor and reattaches
  the missing components, preserving the invariant already enforced by
  `internal/fsguard`. The lexical input may be retained only as audit context;
  policies receive the canonical identity. Failure to obtain that identity is
  deny, and no per-harness interceptor may evaluate the raw path as a fallback.
- Rego/OPA remains the documented fallback if Cedar's younger ecosystem
  (smaller `cedar-go`, no turnkey `cedar test`-equivalent at research time)
  proves insufficient in implementation.

This proposal does not yet supersede the active permission-parity contract.
Until the Cedar evaluator, migration, and acceptance gates land together,
`agm/internal/permissionparity/SPEC.md` and the manifest
`permission_policy` projections remain the authoritative implemented control
plane. Acceptance of this ADR requires updating that SPEC, its executable
surface inventory, and the harness BDD contract in the same migration change;
there must never be two live policy owners.

## Implementation acceptance gates

Before a Cedar interceptor can become a blocking runtime path, executable
tests must prove all of the following:

1. The publisher rejects an otherwise valid bundle whose envelope is unsigned,
   signed by an untrusted identity, or supplied through an agent-writable
   candidate path. The active bundle, version, and durable bytes remain
   unchanged, and harness tool surfaces cannot write the active-bundle
   directory or invoke publication.
2. A malformed or fixture-failing candidate bundle leaves the active
   last-known-good bundle and its version unchanged.
3. Concurrent evaluation during reload observes one whole validated bundle,
   never mixed schema and policy generations.
4. A reload forced between the invocation and confirmation-free decisions does
   not change the request's pinned bundle version; both decisions use the same
   immutable snapshot.
5. A reload that adds an invocation `forbid` while a request waits for
   confirmation invalidates the old confirmation and denies final dispatch.
   A reload at the final dispatch boundary likewise causes reevaluation; tests
   must prove there is no stale-version check/use window.
6. A Cedar response containing both a proven invocation `forbid` reason and
   diagnostics remains `deny`; an invocation response with no proven
   applicable `permit` also remains `deny`. An invocation Allow whose
   diagnostics report an errored policy — including a `forbid` that failed to
   evaluate against an incomplete request entity — also remains `deny` and
   never advances to the confirmation-free decision. Only diagnostics
   encountered after positive invocation authorization may return
   `policy_unavailable`.
   All paths record the bundle version and sanitized diagnostics.
7. Every harness interceptor produces the same canonical resource for
   equivalent absolute, relative, and traversal-containing paths. A worktree
   symlink into a protected source tree is evaluated as the protected target,
   and a missing leaf beneath a symlinked ancestor is resolved through that
   ancestor. Canonicalization failures deny before Cedar is called.
8. Positively invocation-authorized interactive `policy_unavailable` requests
   enter the harness confirmation path; non-interactive requests fail closed
   with a distinct evaluator error.
9. Restart restores the last-known-good bundle before the interceptor accepts
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

If accepted after migration, the persona/policy layer gets one deterministic,
formally-analyzable source of truth instead of N harness-specific permission
configs to keep in sync. Implementation work is scoped as: a Cedar policy schema for
tool/resource/context, a shared evaluation library, and one interceptor per
harness hook point (starting with dear-agent's own pretool hooks). Cedar's
thinner ecosystem and unproven testing tooling (relative to `opa test` or
Cerbos's YAML test suites) are accepted risks, bounded by atomic validation,
last-known-good recovery, and the executable liveness gates above. Rego/OPA is
the fallback if Cedar blocks implementation. The historical `0d1b3ed0` Rego
re-evaluation identifier could not be resolved in the canonical Beads store;
this ADR supersedes that unverified follow-up as the durable decision record.
