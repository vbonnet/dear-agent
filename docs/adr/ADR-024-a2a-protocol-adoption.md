# ADR-024: A2A Protocol as the Agent ↔ Supervisor Fabric

**Status**: Proposed
**Date**: 2026-05-26
**Context**: A spike implementation in `pkg/a2a/` + `pkg/a2a/client/` (this
PR) wraps the upstream `github.com/a2aproject/a2a-go` SDK to expose a
Claude Code session as an A2A HTTP/JSON-RPC endpoint and to drive it from
a supervisor. This ADR captures the *adoption* decision: A2A becomes the
canonical wire protocol between AGM-managed sessions and VROOM
supervisors, and the protocol's `input-required` task state replaces the
in-process `AskUserQuestion` blocking call.

Builds on / aligns with:

- [ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md)
  and [/CONTEXT.md](../../CONTEXT.md) — the 3-supervisor mesh (Meta-O /
  Orchestrator / Overseer) over Primary / Secondary / Tertiary workers
  whose inter-tier communication this protocol carries.
- [agm ADR-019: A2A Agent Cards from Manifests](../../agm/docs/adr/ADR-019-a2a-agent-cards.md)
  — already adopted the A2A *AgentCard* format for session discovery; this
  ADR generalises the choice to the full task protocol.
- [ADR-023: Friction Reporting and Session Handoff](ADR-023-friction-reporting-and-session-handoff.md)
  — friction reports and handoff messages travel as A2A artifacts on the
  same fabric.

---

## Context

dear-agent ships a multi-tier supervisor architecture (Meta-Orchestrator,
Orchestrator, Overseer) over worker tiers (Primary, Secondary, Tertiary)
— see CONTEXT.md and ADR-002. Today, the seam between a supervisor and
the agent it supervises is **ad-hoc**:

- **AGM** drives sessions through tmux: the supervisor writes keystrokes
  into a pane, scrapes output, and infers state from hook events.
- **VROOM** sketches an in-process supervisor loop (`pkg/vroom/supervisor`)
  that runs *inside* the same process tree as the worker, with no
  protocol surface.
- **AskUserQuestion** — the Claude Code tool a worker would naturally
  call to *ask its supervisor anything* — only works against an
  interactive human terminal. Under a supervisor agent it
  **deadlocks**: there is no human at the other end and the supervisor
  has no protocol-level way to observe the pause or answer the
  question. The dear-agent CLAUDE.md already carries the warning ("NEVER
  use AskUserQuestion — it will deadlock") in every supervised session
  prompt, which is documentation paying down for missing wire-protocol
  support.

Three shapes a fabric could take were on the table:

1. **Bespoke JSON-over-stdin** between AGM and the session. Cheap, but
   every consumer (VROOM, MCP servers, external observers) re-implements
   it. Closes the door on multi-host scale.
2. **MCP** (Model Context Protocol). Already in heavy use for tooling. But
   MCP is *tool-shaped*: a client calls named tools that return values.
   It does not model long-running tasks with state transitions, and the
   `tools/list` discovery is not designed for an agent registry. Mapping
   "I am paused waiting for input" onto MCP would mean re-inventing what
   A2A already standardises.
3. **A2A** (Agent-to-Agent protocol — https://a2a-protocol.org). A spec
   for agent-to-agent communication over JSON-RPC (and gRPC) with:
   - An **AgentCard** discovery document at `/.well-known/agent-card.json`.
   - A **Task** lifecycle (submitted → working → input-required →
     completed | failed | canceled) carried as `TaskStatusUpdateEvent`s.
   - A first-class `input-required` state for "this task is paused
     waiting for a follow-up message from the client."
   - An official Go SDK we already vendor (`a2aproject/a2a-go` —
     present in `go.mod` since the AGM ADR-019 work) and which is in
     active development by Google's A2A team.

The spike confirms the third option is workable end-to-end: a four-test
integration suite in `pkg/a2a/integration_test.go` exercises a complete
input-required round-trip in <400ms on a laptop, with no central queue,
no per-process cap, and one HTTP listener per agent.

---

## Decision

### D1. A2A is the canonical agent ↔ supervisor wire protocol

Every supervisor-to-agent interaction that crosses a process boundary
SHOULD use A2A. Concretely:

- An AGM-managed session that wants to be observable by a supervisor
  exposes itself as an A2A endpoint via `pkg/a2a.Server`.
- A VROOM supervisor that wants to delegate work to a session uses
  `pkg/a2a/client.Client.Send` rather than tmux keystroke injection or
  in-process function calls.
- Discovery is by URL — either a hard-coded endpoint or by resolving
  the agent's well-known `AgentCard` (built by `pkg/a2a.SessionCard`).

The existing AGM AgentCard generator (`agm/internal/a2a`) keeps doing
exactly its job: it produces the *card document* from session manifest
metadata. This ADR adds the *server* and *client* halves that the card
already advertises.

### D2. `input-required` replaces in-process `AskUserQuestion`

When a worker needs supervisor input, the path is:

1. The worker calls `SessionIO.AskInput(ctx, prompt)`.
2. The session emits a `TaskStatusUpdateEvent{State: TaskStateInputRequired, Final: true}`
   carrying the prompt.
3. The supervisor's `client.Client.Send` returns from its
   blocking `SendMessage`, observes the input-required state, and
   invokes the configured `AnswerFunc`.
4. The supervisor decides locally, escalates up its own chain, or
   surfaces to a human — whichever the policy dictates — and replies
   with a follow-up `message/send` carrying the same `TaskID`.
5. The worker's `AskInput` call returns with the answer; execution
   resumes from the next line.

This is the **protocol-native** answer to the AskUserQuestion deadlock.
The worker is never blocked on a non-existent terminal; the supervisor
sees the pause as a normal task-state transition; and the same
mechanism works whether the supervisor is in-process, on the same host,
or one hop away on a different machine.

The dear-agent CLAUDE.md instruction "NEVER use AskUserQuestion" stays
in force *for the current Claude Code tool of that name*. The
replacement is `SessionIO.AskInput`, which is wire-protocol-aware and
does not deadlock under supervision.

### D3. Horizontal scaling is achieved by N independent servers, not by sharding

Each `pkg/a2a.Server` is single-tenant: one HTTP listener, one
`SessionHandler`, one AgentCard. To scale:

- Run N sessions → bind N listeners (or N processes on N hosts).
- A supervisor that orchestrates K agents holds K `client.Client`s and
  K TaskIDs. Each task is its own conversation; there is no shared
  state on the supervisor side except whatever the supervisor itself
  keeps about who is doing what.
- The upstream SDK exposes a `ClusterConfig` for shared task storage
  across replicas of *the same agent* (see
  `a2asrv.WithClusterMode`). We deliberately defer that knob — single
  Claude Code sessions are not sharded.

There are no caps in `pkg/a2a` itself. Concurrency is bounded by OS
file-descriptor limits and by whatever the underlying SessionHandler is
willing to do, not by an artificial in-package constant.

### D4. The spike's surface is the v1 API shape

The spike commits these public types:

| Package           | Type / Function                                       | Role |
|-------------------|-------------------------------------------------------|------|
| `pkg/a2a`         | `Server`, `ServerConfig`                              | Bind a listener + JSON-RPC handler + AgentCard mux. |
| `pkg/a2a`         | `SessionHandler` / `HandlerFunc`                      | Application hook called per task. |
| `pkg/a2a`         | `SessionIO` (`Emit`, `AskInput`, `TaskID`, `ContextID`) | Runtime interface while a task is in flight. |
| `pkg/a2a`         | `SessionCard`                                         | Builder for `*a2a.AgentCard` with dear-agent defaults. |
| `pkg/a2a/client`  | `Client`, `NewFromEndpoint`, `NewFromCardURL`, `Send` | Supervisor-side driver loop. |
| `pkg/a2a/client`  | `AnswerFunc`                                          | Policy hook invoked per input-required pause. |

The shape is intentionally narrow: a handler is "process a prompt, maybe
ask for input, return". Anything fancier (streaming, push notifications,
artifact uploads, gRPC transport) is supported by the underlying SDK and
can be exposed in pkg/a2a later without breaking the v1 callers.

---

## Consequences

### Positive

- **Deadlock-free supervision.** The class of "the worker called
  AskUserQuestion and now nothing is listening" failures stops being a
  CLAUDE.md instruction the model has to remember and starts being a
  protocol fact the runtime guarantees.
- **One fabric, many consumers.** AGM, VROOM, external observers,
  third-party agents (anything that speaks A2A) all converge on the same
  surface. The AGM cards generator (ADR-019) and the new server here
  are now two halves of the same story, not parallel concerns.
- **Multi-host by default.** A supervisor on host A can drive a
  session on host B with no code change — the transport is HTTP. This
  unblocks the "VROOM on a control plane, workers in worktrees on the
  developer's machine" topology sketched in ADR-002.
- **Free observability.** Every `TaskStatusUpdateEvent` is a structured
  record of the conversation, suitable for the VROOM decision trail and
  for the friction-reporting stream of ADR-023 without bespoke
  serialisation.

### Negative

- **Network on the inside.** A session that was a tmux pane and a few
  goroutines now also speaks HTTP, even when supervisor and worker are
  on the same machine. The cost is small (loopback, JSON-RPC, ~1ms
  per call) but it is non-zero and it adds a port-allocation concern
  for environments where 65k ephemeral ports is not abundant. Mitigation:
  the spike uses `127.0.0.1:0`; nothing leaves the host unless an
  explicit configuration opens it up. Unix-socket transport is on the
  SDK roadmap for the inevitable "I want zero ports" case.
- **Two ways to ask, one of which still exists.** The Claude Code
  `AskUserQuestion` tool keeps existing; it is the right tool for
  interactive *human* terminals and is what `claude` uses without a
  supervisor. Workers must therefore distinguish: "I am being
  supervised → use `SessionIO.AskInput`" vs "I am interactive → use
  AskUserQuestion." The CLAUDE.md "NEVER AskUserQuestion under
  supervision" line continues to be the discriminator. A future ADR may
  fold this into a single facade.
- **One more dependency to keep current.** `a2aproject/a2a-go` is on
  v0.3.x and pre-1.0; the protocol itself is at v0.3.0. We accept the
  churn risk as the price of not building a bespoke equivalent. The
  surface we depend on (Server, Handler, Client, Card) is the most
  stable part of the SDK.

### Neutral

- The existing `agm/internal/a2a` package (cards generator) is not
  touched by this ADR. It remains the authoritative source of *card
  content* derived from the session manifest. `pkg/a2a.SessionCard` is a
  thin builder for code paths that do not have a manifest; the two are
  complementary, not duplicative. A follow-up may consolidate them.

---

## Alternatives Considered

### A. Stay with tmux + hook-based state detection (status quo)

Rejected. The substrate works for AGM's single-session control loop but
falls apart the moment a supervisor needs to *answer* a worker's
question: there is no event-shaped channel back. Every "the agent got
stuck waiting for input" retro in `docs/retros/` is a tax this option
keeps paying.

### B. Build a bespoke supervisor RPC

Considered. We could ship a thin JSON-over-WebSocket protocol with
exactly the message types we need. Faster to v1, but loses two things
A2A buys for free: (i) discovery via the well-known AgentCard, which
ADR-019 has *already* committed to; (ii) interoperability with any
third-party agent or observer that speaks A2A. The cost of A2A's full
surface is mostly imports — the actual code added in the spike is
~500 lines, less than a bespoke equivalent would need.

### C. Use MCP as the supervisor protocol

Rejected. MCP is tool-shaped: stateless name-and-args invocations
returning values. Modelling "this task is paused at input-required for
12 minutes while the supervisor decides" inside MCP would mean
inventing a task state machine on top of `tools/call`. A2A *is* that
state machine, standardised. MCP remains the right answer for *tool*
access (file I/O, search, code execution) and we keep using it there.

### D. Adopt A2A's gRPC transport instead of JSON-RPC

Deferred. The SDK supports both. JSON-RPC was picked for the spike
because (i) it is the default the A2A AgentCard advertises across SDKs;
(ii) loopback HTTP is one less moving piece than a gRPC stack; (iii)
the supervisor / worker call rate is bounded by *human-scale* events
(an `AskInput` every few seconds at most), not by sub-millisecond
streaming. We can add gRPC as an additional interface on the AgentCard
when a workload demands it.

---

## Open Questions (Tracked, Not Blocking)

1. **Authentication.** The spike serves loopback with no auth. A2A
   supports OAuth2, API keys, and mTLS via `SecuritySchemes` on the
   AgentCard. A follow-up ADR will pick the dear-agent default — most
   likely "loopback unauthenticated, anything else requires mTLS or a
   shared bearer."
2. **Persistence of paused tasks.** The spike keeps the handler
   goroutine alive in process memory while a task is in `input-required`.
   A process restart loses parked tasks. The SDK ships persistent
   `TaskStore`/`WorkQueue` implementations (the `examples/clustermode`
   reference). Wiring those is the next step if we want supervisor
   restarts to be transparent.
3. **Folding `AskUserQuestion` and `AskInput` behind a single facade.**
   The two callers differ only in destination. A small `pkg/ask`
   shim could pick based on session metadata. Worth a separate ADR
   once `pkg/a2a` has at least one production caller.

---

## Implementation Note

The spike at `pkg/a2a/` and `pkg/a2a/client/` is the reference. Future
features should extend that surface rather than fork it; in particular
the goroutine-per-task lifecycle and the channel-based pause/resume in
`sessionExecutor` are load-bearing for the `input-required` semantics
and should not be reshaped without an update here.
