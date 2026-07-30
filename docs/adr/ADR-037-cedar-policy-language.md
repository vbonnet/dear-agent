# ADR-037: Cedar as the persona/policy-enforcement language

Status: Proposed (2026-07-27; implementation and migration pending)

## Context

Stage 1 of the agent-persona-system research
(`research/2026-07-21-agent-persona-system-research.md`, engram-research PR
#163) left open how tool and resource policy should be authored and enforced
across harnesses. Its companion policy-language research evaluated
EARS/Gherkin, Rego/OPA, CEL, Cerbos, and Cedar for runtime interception of a
live principal/action/resource/context request, not spec-time verification.

EARS/Gherkin operate one layer up and never evaluate a live request. Rego/OPA
is the most mature runtime candidate, but authorization-shaped systems are
moving away from its "Rego tax" (Kyverno off Rego; Kubernetes toward CEL).
Cerbos has strong built-in testing but its service-first design fights an
embeddable, in-process, per-harness interceptor.

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
- Put one versioned, typed **tool-action projector** before Cedar; its shared
  registry owns aliases/arguments and emits typed action/resource-role pairs
  plus context. Sources, destinations, and parents retain their operation roles;
  every pair is separately authorized and all must allow; raw maps and adapter-local aliases are forbidden.
  TCP identity includes canonical hostname, resolved IP, and port; hostname policy requires an application-aware trusted proxy enforcing approved SNI/HTTP Host plus IP/port, while socket-only enforcement is IP-granular.
  Every DNS answer/redirect is separately authorized and dispatch binds the approved tuple. Local IPC emits typed `local-socket/connect` plus canonical filesystem/abstract-namespace path and socket identity, bound at connect;
  an unpinned endpoint or protected AGM, tmux, Docker, or control socket denies. Pi `find`/`Glob` map to search, `ls` to directory-list, and `path`/`file_path`
  normalize; multi-resource calls emit all resources; invalid/lossy TCP, local-socket, or file shapes deny before Cedar.
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
  `policy_unavailable`. Escalation requires a nonce receipt bound to the request, policy/input digests, expiry, approver identity/capability, and presentation digest.
  The trusted channel renders principal, every typed action/resource-role pair, all decision-relevant context/effect flags, and versions as length-delimited canonical escaped bytes, never raw terminal text;
  any display/request/receipt, resource-role, context, or approver-scope mismatch denies. Tmux input, harness callbacks, and agent-addressable messages cannot prove approval;
  without an integrity-protected channel, confirmation fails closed. A harness-independent pre-receipt deadline makes silence/cancellation return typed
  `confirmation_timeout`/`confirmation_cancelled`, execute nothing, and release the request; receipt expiry is separate and finalization compares it with a trusted clock. Diagnostics never make a
  forbid or missing permit user-overridable. Final dispatch checks expiry and uses a privileged nonce-consumption authority outside every agent-writable root; durable atomic consumption lets one contender execute and denies replay.
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
  supported home forms, cleans traversal, and makes paths absolute. A relative
  path is anchored to the verified effective cwd when its consuming command
  runs, including prior shell `cd`; the initial interceptor cwd is insufficient.
  Unprovable cwd/control flow makes the resource unprojectable. Object reads or
  opens resolve symlinks; missing leaves resolve the deepest existing ancestor
  and reattach components, preserving the `internal/fsguard` invariant.
- Directory-entry operations use a no-follow projection instead. `unlink`,
  replacement, and each side of `rename` authorize the canonical parent
  directory plus the leaf entry without resolving the leaf symlink; rename
  authorizes both source and destination parent/entry resources. If an
  operation also reads or mutates the referent, that referent is a separate
  required resource rather than a substitute for the directory entry. Thus a
  symlink to an allowed temporary file cannot authorize deletion from a
  protected directory, and a protected referent does not by itself forbid
  removing an otherwise allowed symlink entry.
- Hard links authorize the source identity according to the requested follow
  mode plus destination parent/entry: `ln -L` resolves and pins the referent,
  while `ln -P` no-follow pins the symlink inode. An allowed destination cannot
  launder a protected inode. Later access carries device/inode into authorization.
  The privileged boundary scans immutable protected roots and atomically catalogs
  trusted mutations; an uncorrelated external mutation makes the catalog
  incomplete and denies ambiguous access until rescan.
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
`permission_policy` projections, plus `internal/fsguard` policy/configuration and the
pretool Bash/filesystem guard hooks, SPECs, and tests remain authoritative.
Acceptance requires updating or retiring that complete inventory and the
harness BDD contract in one migration; there must never be two policy owners.

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
5. A reload adding an invocation `forbid` while a request waits for confirmation
   invalidates it and denies final dispatch. Bundle or authoritative-input
   revision changes at the final dispatch boundary cause reevaluation for both
   direct `allow` and `ask`; tests prove no stale snapshot check/use window and
   that revoked direct authorization cannot dispatch. Role/entity/context
   changes without policy reload also invalidate confirmation and rerun both
   decisions. Agent-authored role/context data in a writable manifest is
   rejected; only authenticated control-plane updates advance the input revision.
6. A Cedar response containing both a proven invocation `forbid` reason and
   diagnostics remains `deny`; an invocation response with no proven
   applicable `permit` also remains `deny`. An invocation Allow whose
   diagnostics report an errored policy — including a `forbid` that failed to
   evaluate against an incomplete request entity — also remains `deny` and
   never advances to the confirmation-free decision. Only diagnostics
   encountered after positive invocation authorization may return
   `policy_unavailable`.
   All paths record the bundle version and sanitized diagnostics.
7. Every harness produces the same typed action/resource-role pairs/context for
   equivalent aliases and inputs; invalid or missing input fails before Cedar.
   A copy from a denied source to an allowed destination evaluates both roles and the all-of result denies dispatch; an all-allowed multi-resource copy
   evaluates every source, destination, and parent pair and dispatches. A live allowed hostname dispatches while a denied co-hosted Host/SNI and
   private/control-plane tuple stay blocked across DNS, redirect, and rebinding; socket-only TCP mode proves IP-granular semantics. Each harness
   denies canonical AGM, tmux, and Docker local-socket resources and dispatches to an explicitly allowed test socket; endpoint replacement between
   evaluation and `connect` cannot redirect the operation. Equivalent filesystem paths unify and protected symlink/missing-leaf targets deny before Cedar.
8. After positive invocation authorization, every harness drives an authored
   confirmation-free Deny: interactive mode enters `ask`, while non-interactive
   mode fails closed and dispatches nothing. Positively authorized interactive
   `policy_unavailable` requests may also enter confirmation; non-interactive
   requests fail closed. Every harness forces panic, crash, signal, nonzero
   exit, EOF, timeout, and incomplete responses at each gate. Before positive
   invocation authorization these return typed non-confirmable unavailability
   by the deadline and execute nothing; only later confirmation-free failure
   may escalate. Every harness rejects an authenticated but unauthorized signer and any mismatch among displayed canonical inputs, context, request, and
   receipt; changing only a resource role or context (such as a force flag) rejects approval. Paths, hostnames, and context containing newlines, bidi controls,
   terminal escapes, delimiters, or lookalike bytes render unambiguously and cannot produce approval for different canonical bytes. Terminal keys, callbacks, and
   agent messages cannot mint a human receipt. Trusted display and approval of unchanged snapshots reauthorizes one dispatch; finalization under a trusted clock
   rejects a receipt delayed beyond expiry even when its signature/digests remain valid. A privileged nonce authority accepts one duplicate/concurrent receipt;
   store deletion/rollback, replay, expired receipt, silence, and cancellation dispatch nothing.
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
14. Hard-link tests exercise source symlinks in both modes: `-L` authorizes
    the referent and `-P` the symlink inode, with outcomes bound to that identity.
    They deny protected-source/allowed-destination, the inverse, and protected-inode aliases; a complete catalog permits fully allowed aliases.
    An external uncorrelated link makes it incomplete and fail closed until
    rescan; pathname allow never overrides protected-inode identity.
15. Every harness runs an unrecognized binary with runtime-computed file, TCP,
    local-socket, and `cd` targets. Opaque/computed protected reads/writes,
    private/control-plane connections, and AGM/tmux/Docker control sockets are
    blocked by the sandbox/socket/trusted-proxy boundary or denied before launch,
    while an approved external endpoint and explicitly allowed test socket prove
    liveness. A projectable `git status --short` dispatches; raw-string/post-hoc,
    opaque-all, or reject-all enforcement does not pass.

The shared evaluator SPEC and per-harness interceptor BDD scenarios must carry
these cases; unit tests of Cedar Allow/Deny alone do not satisfy this gate.

## Alternatives

**Rego/OPA** — rejected despite greater maturity because Cedar is purpose-built
for this shape and the newest on-point AWS/Microsoft prior art chose it.
**CEL** — rejected as a canonical layer because it is an expression primitive,
not a policy system. Reconsider it inside a bespoke format if needed.
**Cerbos** — rejected because its service-first posture mismatches in-process
interceptors; revisit if a centralized policy service becomes desirable.

**Per-harness native-config codegen** (compile canonical policy directly
into each harness's own settings format) — rejected; no researched
prior art actually does this, and the shared-interceptor pattern already
matches Stage 1's own finding for the Pi adapter.

## Consequences

After migration, the persona/policy layer gets one deterministic source of
truth instead of N harness configs. Work comprises a Cedar schema, shared
evaluator, and interceptor per harness hook. Cedar's thinner ecosystem and
unproven testing tooling are bounded by atomic validation, last-known-good
recovery, and the gates above; Rego/OPA remains fallback. The historical
`0d1b3ed0` Rego follow-up was absent from canonical Beads, so this ADR
supersedes that unverified reference.
