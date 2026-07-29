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
- Treat the interceptor executable and its harness registration as privileged
  enforcement assets, not project files. Each blocking integration must load
  both from an operator-managed location outside the workspace and every
  agent-writable root, authenticate their expected identity at startup, and
  fail closed if either changes or disappears. Project-local registrations
  such as `.codex/hooks.json` or `.claude/settings.json` may remain advisory,
  but they cannot be the sole Cedar enforcement boundary. A harness that
  cannot provide an integrity-protected registration or dispatcher boundary
  does not qualify for blocking Cedar enforcement.
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
  invocation authorization may confirmation-free diagnostics produce
  `policy_unavailable`. Escalation requires a nonce-bearing approval receipt
  bound to the request, policy/input digests, authenticated human identity,
  expiry, and an out-of-band channel that agent tools cannot write. Tmux input,
  harness UI callbacks, and agent-addressable messages are not proof of human
  approval; without an integrity-protected channel, confirmation is unavailable
  and execution fails closed. Diagnostics never make a forbid or missing permit
  user-overridable.
- Bound every evaluator call by a harness-independent deadline. A panic,
  process crash, signal, nonzero exit, EOF, transport closure, timeout,
  cancellation without a complete decision, or incomplete response is a typed
  evaluator-unavailable result that executes nothing. Failure before positive
  invocation authorization is never user-confirmable because no permit was
  proven. Failure at the confirmation-free gate may use explicit escalation
  only after the same snapshots produced positive invocation authorization.
  Harness adapters may translate presentation but never fail open or wait forever.
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
- Acquire immutable bundle and authorization-input snapshots before invocation.
  Inputs include principal, entity graph/memberships, canonical resources,
  context, stable revision/digest, and authenticated provenance. Their durable
  source and writer credentials live outside every agent-writable root;
  manifests or `~/.agm` state writable by a harness are never authoritative.
  Reuse the exact snapshots for confirmation-free evaluation. After `ask`, the
  dispatcher reacquires both at final dispatch; a version, digest, or provenance
  change invalidates approval and reruns both decisions. `deny` stops, `allow`
  proceeds, and `ask` needs a new receipt bound to both snapshots.
- Serialize the atomic final version/input check and consumption of an
  authorization against bundle publication and authorization-input revision
  changes. Do not hold that finalization lease while waiting for a person.
  If churn invalidates a confirmation repeatedly, stop after a configured
  finite retry limit and return a distinct retryable `policy_churn` unavailable
  result that executes nothing. An unbounded restart loop is not conforming:
  every request must dispatch, deny, or return a typed unavailable result in
  bounded attempts.
- Project every path-bearing resource into operation-specific canonical
  filesystem identities before Cedar evaluation. The shared projector expands
  supported home forms, anchors relative paths to the interceptor's verified
  working directory, cleans traversal components, and makes the path absolute.
  Operations that consume the target object, such as read or ordinary open,
  resolve symlinks and authorize the referent. For a missing leaf they resolve
  the deepest existing ancestor and reattach the missing components, preserving
  the invariant already enforced by `internal/fsguard`.
- Directory-entry operations use a no-follow projection instead. `unlink`,
  replacement, and each side of `rename` authorize the canonical parent
  directory plus the leaf entry without resolving the leaf symlink; rename
  authorizes both source and destination parent/entry resources. If an
  operation also reads or mutates the referent, that referent is a separate
  required resource rather than a substitute for the directory entry. Thus a
  symlink to an allowed temporary file cannot authorize deletion from a
  protected directory, and a protected referent does not by itself forbid
  removing an otherwise allowed symlink entry.
- Hard links authorize the source identity plus destination parent/entry; an
  allowed destination cannot launder a protected inode. Later access carries
  device/inode into authorization. At startup the privileged boundary scans
  protected roots that agents cannot mutate, then serializes trusted mutations
  with atomic catalog updates. An uncorrelated external mutation marks that
  filesystem catalog incomplete and denies inode-ambiguous access until rescan.
  With a complete catalog, multiply-linked inodes absent from it remain usable.
- The lexical input may be retained only as audit context; policies receive
  the operation-appropriate canonical identities. Failure to obtain every
  required identity is deny, and no per-harness interceptor may evaluate the
  raw path as a fallback. Authorization must remain bound to those same
  filesystem objects through execution: existing referents are consumed
  through race-resistant handles, while entry operations execute relative to
  pinned parent-directory handles with no-follow semantics. Creation beneath a
  missing leaf likewise uses a pinned existing ancestor. The dispatcher must
  never authorize one identity and then reopen the caller's lexical path.
  Where a harness cannot pass such handles, an independently enforced OS
  sandbox must constrain the final operation to every authorized identity;
  otherwise the operation is denied. A path-string check followed by an
  ordinary open, unlink, or rename is not a conforming interceptor.
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
   must prove there is no stale-version check/use window. Separately, changing
   a principal's role/entity membership or any authorization context without a
   policy reload invalidates the old confirmation and reruns both decisions
   against the new input snapshot. Agent-authored role/context data in a
   writable manifest is rejected; only authenticated control-plane updates can
   advance the authoritative input revision.
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
   enter confirmation; non-interactive requests fail closed. Every harness
   forces panic, crash, signal, nonzero exit, EOF, timeout, and incomplete
   responses at each gate. Before positive invocation authorization these
   return typed non-confirmable unavailability by the deadline and execute
   nothing; only later confirmation-free-gate failure may enter escalation.
   Tests inject terminal keys, harness callbacks, and agent messages while an
   `ask` is pending and prove none can mint the required human receipt.
   A positive end-to-end case proves the authenticated out-of-band human
   channel can mint that receipt and, with unchanged bundle, input, principal,
   action, resource, and filesystem snapshots, final reauthorization permits
   exactly one dispatch.
9. Restart restores the last-known-good bundle before the interceptor accepts
   tool calls.
10. For every harness, replacing or removing the interceptor executable or its
    registration from an agent-writable session cannot disable enforcement.
    The tamper attempt either has no effect because the assets are outside the
    writable boundary or makes startup/tool dispatch fail closed; tests must
    cover both the executable and registration.
11. A concurrent process that swaps an authorized target or any writable
    ancestor for a symlink between projection and dispatch cannot redirect the
    operation into a protected tree. Tests must exercise existing targets and
    missing-leaf creation and prove execution consumes the pinned object or
    ancestor rather than reopening the lexical path.
12. Sustained bundle-publication or authorization-input churn cannot keep a
    request in an internal retry loop indefinitely. Tests force more
    invalidations than the configured limit and observe a typed
    `policy_churn` result, no dispatch, and a bounded evaluation count; a
    non-churning request still makes progress while publications occur.
13. Entry mutations use no-follow projection: tests unlink an allowed-directory
    symlink with a protected referent, reject a protected-directory symlink
    with an allowed referent, and cover rename/replacement across independently
    authorized parents. Leaf swaps cannot change pinned entries or parents.
14. Hard-link tests deny protected-source/allowed-destination and the inverse,
    plus pre-existing allowed-path aliases of protected inodes. A complete
    catalog permits allowed-source/allowed-destination aliases; an external
    uncorrelated link makes it incomplete and fail closed until privileged
    rescan. Pathname allow never overrides protected-inode identity.

The shared evaluator SPEC and per-harness interceptor BDD scenarios must carry
these cases; unit tests of Cedar Allow/Deny alone do not satisfy this gate.

## Alternatives

**Rego/OPA** — rejected despite greater maturity because Cedar is purpose-built
for this authorization shape and the newest on-point AWS/Microsoft prior art
chose Cedar over it.

**CEL** — rejected as a canonical authoring layer because it is an expression
primitive, not a policy system. Reconsider it inside a bespoke format if needed.

**Cerbos** — rejected because its service-first posture mismatches in-process
interceptors. Revisit only if a centralized policy service becomes desirable.

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
