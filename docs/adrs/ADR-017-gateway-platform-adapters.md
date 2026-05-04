# ADR-017: Gateway and Platform Adapters

**Status**: Proposed
**Date**: 2026-05-03
**Context**: dear-agent ships several user-facing surfaces — the workflow CLIs
(`workflow-run`, `workflow-status`, etc.), the MCP server (`dear-agent-mcp`),
and the Tailscale HTTP API ([ADR-013](ADR-013-tailscale-api.md)). Each surface
re-implements the same wiring: parse a request, look up the caller, dispatch
into the workflow engine, marshal a response. The HermesAgent research
(recommendation #4) flags this duplication as the next thing to consolidate
before adding more surfaces (mobile, web, chat platforms). This ADR introduces
a **Gateway** — a transport-agnostic message bus that owns dispatch — plus an
**Adapter** interface that platform-specific I/O implements. Two adapters
(CLI, HTTP) ship with this ADR; chat platforms (Discord, Slack, Matrix) are
defined as the next consumers but not built here.

Builds on:

- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
  — the engine's `runs.db` is the durable substrate; the gateway is a thin
  routing layer over it, never a parallel state store.
- [ADR-012: Provider Transport Layer](ADR-012-provider-transport-layer.md)
  — the LLM provider router is the model for what we want here at the
  inbound transport layer: one core, many pluggable surfaces.
- [ADR-013: Tailscale API](ADR-013-tailscale-api.md) — the HTTP surface
  this ADR refactors becomes the first non-CLI gateway adapter.

---

## Context

Three concrete pains drove this ADR:

1. **Per-surface wiring drift.** `pkg/api` validates `RunRequest.File` for
   shell metacharacters; `cmd/dear-agent-mcp` re-implements its own
   validation for the MCP `workflow_run` tool. Adding a third surface
   means a third copy. Every surface also has its own caller-identity
   shim (Tailscale `WhoIs`, env var, MCP session id) plumbed differently
   into `audit_events.actor`.
2. **No path for asynchronous events.** The HTTP API is request/response
   only. When a HITL gate opens during a run, the only way a caller
   learns is by polling `GET /gates`. A chat-platform adapter (Discord
   bot that DMs the on-call engineer when a gate opens) has no event
   stream to subscribe to. Each surface that needs events would have to
   tail `runs.db` directly.
3. **Coupling of intent to transport.** A Discord adapter that accepts
   `!run my-workflow.yaml inputs.foo=bar` shouldn't have to construct an
   HTTP request to its own host or invoke the workflow runner binary
   directly. It needs an in-process call surface that owns
   authentication, validation, and audit, identical to what HTTP and MCP
   already do — but packaged once.

The substrate is solid: `runs.db` is the durable record, `pkg/workflow`
is the canonical Go API, `pkg/audit` is the canonical write log. What's
missing is an in-process dispatch layer between "user intent expressed
in some platform's format" and "engine call." That layer is the Gateway.

---

## Decision

Introduce `pkg/gateway/` — an in-process, transport-agnostic message
bus. Three message types, one dispatcher, one adapter interface, two
built-in adapters (CLI, HTTP).

### D1. Three message types: Command, Response, Event

```go
// Command is a user intent: "run this workflow", "approve this gate",
// "fetch this status." Adapters parse their wire format into a Command;
// the gateway dispatches it.
type Command struct {
    ID     string         // adapter-assigned correlation id
    Type   CommandType    // run | status | approve | reject | cancel | list
    Caller Caller         // identity (login_name + display)
    Args   map[string]any // type-specific arguments
}

// Response is the synchronous reply to a Command. One Response per
// Command. Body is the type-specific payload; on error, Err is set
// and Body is nil.
type Response struct {
    CommandID string         // matches Command.ID
    Body      map[string]any
    Err       *Error         // structured error with Code + Message
}

// Event is asynchronous: "a run finished", "a HITL gate opened". The
// gateway broadcasts events to every subscribed adapter; whether the
// adapter forwards the event to its end user is the adapter's choice.
type Event struct {
    Type    EventType      // run_finished | hitl_opened | hitl_resolved | run_failed
    Subject string         // run_id, approval_id, etc.
    Payload map[string]any // event-specific
}
```

Args/Body/Payload are `map[string]any` rather than typed structs because
the dispatcher routes to handlers that already own the canonical typed
shape — `pkg/workflow.RunStatus`, `pkg/workflow.HITLRequest`, etc. Each
handler unmarshals what it needs and re-marshals into the response map.
This keeps the gateway small and lets handlers evolve independently.

The single error path is structured (`gateway.Error{Code, Message}`) so
adapters can map `code` to their wire format: HTTP status, JSON-RPC code,
chat ack symbol.

### D2. Gateway is a registry of handlers, plus a fan-out for events

```go
type Gateway interface {
    Dispatch(ctx context.Context, cmd Command) Response
    Subscribe(adapter Adapter) (unsub func())
    Publish(ev Event)
}
```

A `Gateway` is constructed with handlers registered for each
`CommandType`. `Dispatch` looks up the handler and calls it. Unknown
command types return `ErrUnknownCommand`. Adapters that want to forward
events call `Subscribe`; the returned `unsub` removes them. `Publish` is
called by handlers (or by a future `runs.db` tailer) when something
asynchronous happens.

Handler signatures are uniform:

```go
type Handler func(ctx context.Context, cmd Command) Response
```

The default handler set wraps `pkg/workflow`:

| CommandType | Handler                                     |
|-------------|---------------------------------------------|
| `run`       | spawn workflow-run via the existing Runner  |
| `status`    | `workflow.Status`                           |
| `list`      | `workflow.List`                             |
| `logs`      | `workflow.Logs`                             |
| `gates`     | `workflow.ListPendingHITLRequests`          |
| `approve`   | `workflow.RecordHITLDecision(approve)`      |
| `reject`    | `workflow.RecordHITLDecision(reject)`       |
| `cancel`    | (Phase 2 placeholder; calls workflow.Cancel)|

The handler set is a struct of fields, not a global; tests construct
their own gateway with stub handlers. Registration is explicit (no
reflection, no init-time magic).

### D3. Adapter interface — per-platform I/O

```go
type Adapter interface {
    // Name identifies the adapter in logs and metrics. Lowercase,
    // dotless: "cli", "http", "discord".
    Name() string

    // Run blocks until ctx is cancelled or the underlying transport
    // closes. The adapter reads from its transport, builds Commands,
    // and calls gw.Dispatch; it writes the resulting Response back to
    // the transport. Adapters that subscribe to events write them to
    // the transport too.
    Run(ctx context.Context, gw Gateway) error
}
```

This is intentionally broad — an adapter can be stdio-based (CLI), HTTP-
based (the Tailscale API), websocket, gRPC, Slack RTM, Matrix sync, etc.
The contract is one method that owns the per-transport main loop and
calls into the gateway via the public interface.

### D4. Built-in adapters

Two adapters ship in this ADR:

**`pkg/gateway/adapters/cli`** — reads JSON `Command` envelopes from
`io.Reader` (one per line, or one full request if the reader is bounded),
writes JSON `Response` envelopes to `io.Writer`. The shape is

```jsonc
// stdin
{"id":"1","type":"status","args":{"run_id":"abc"}}
// stdout
{"command_id":"1","body":{"run_id":"abc","state":"running","...":"..."}}
```

It's the moral equivalent of a JSON-RPC server but without the JSON-RPC
envelope conventions — those are appropriate for MCP, where the protocol
is fixed; the CLI adapter is what a script or Cron job would speak. The
adapter does not own caller identity itself: it reads the caller from a
constructor argument (defaults to the OS user). Use cases:

- A `dear-agent-cli` binary that reads a single command from argv,
  serializes it as JSON, and reuses the adapter's parser.
- A development REPL that pipes commands at the gateway without
  spinning up HTTP.
- Tests: feed canned input, assert canned output.

**`pkg/gateway/adapters/http`** — wraps an existing `*api.Server`. The
HTTP adapter doesn't replace ADR-013's tsnet integration; it provides
the same `*api.Server` with its `Runner` swapped for a thin shim that
calls `gw.Dispatch(Command{Type: "run", ...})`. The HTTP routes,
identity layer, and validation stay in `pkg/api`. The point of this
adapter is to **route HTTP traffic through the gateway** so dispatch
decisions (rate limits, audit, future caller-based authorization) live
in one place.

### D5. What this ADR explicitly does not build

- **Discord, Slack, Matrix adapters.** The interface is locked but
  implementations are deferred. A separate ADR (or follow-up PR) per
  platform will add them; each gets its own credentials/config story.
- **Bidirectional streaming for `tools/call`-style RPC.** The MCP server
  stays a separate binary in this ADR. We may unify later — `pkg/gateway`
  could grow an MCP adapter — but not before the chat-platform adapters
  prove the abstraction.
- **Persistence of events.** `Publish` is fire-and-forget. Adapters that
  need durability subscribe and write to their own store. The gateway
  refuses to grow into an event log because `runs.db` already is one.
- **Cross-process gateway.** The gateway is in-process. A multi-process
  topology (one gateway, many adapter processes) would require an RPC
  surface; that's a separate ADR if and when we need it.

### D6. Caller identity flows from adapter to handler unchanged

Each adapter is responsible for producing a `Caller` for every command
it constructs:

- CLI adapter: `os/user.Current()` by default; constructor lets tests
  inject a fixed Caller.
- HTTP adapter: delegates to the existing `api.Identifier` (Tailscale
  WhoIs in production).
- Future Discord adapter: maps the Discord user id to a configured
  login name via a static yaml mapping (out of scope here).

The gateway never invents a caller. Handlers stamp `Caller.LoginName`
into `audit_events.actor` exactly as they do today.

---

## Consequences

### Positive

- **One place to add cross-cutting policy.** Rate limiting, dispatch
  tracing, future caller-based authorization, all land in
  `Gateway.Dispatch` once and apply to every adapter.
- **New surfaces reduce to "implement Adapter."** A Discord bot is
  ~150 lines: open the websocket, parse `!run` lines into Commands,
  forward Responses as DMs, subscribe to HITL events. No engine
  knowledge required.
- **Test surface shrinks.** Gateway tests use stub handlers; adapter
  tests use a stub gateway. Most engine wiring is exercised through
  `pkg/workflow` tests already.
- **Audit consistency.** Every command flows through the same dispatch,
  so any future "every command gets logged with caller, type, args" is
  one wrap of `Dispatch`, not eight surface patches.

### Negative

- **Indirection cost in the HTTP path.** A run request now goes
  HTTP handler → adapter shim → `Dispatch` → handler → workflow runner.
  Compared to the current direct handler call, that's two extra
  function calls and one map allocation per request. At HTTP rates this
  is invisible; the cost is conceptual ("where do I look to debug a
  failing /run?"), not performance.
- **Map-shaped Args/Body increases the chance of typos.** A handler
  reading `cmd.Args["run_id"]` won't be flagged by the compiler if a
  caller writes `cmd.Args["runID"]`. We mitigate with a small set of
  centralised arg-key constants per command type and unit tests that
  assert the contract.
- **Two ways to call the engine** during the migration window — direct
  (`workflow.Status` from the MCP server) and via the gateway (HTTP and
  CLI). We accept this. The MCP server is a separate process and
  rewriting it as a gateway adapter is a follow-up; this ADR doesn't
  block on that work.

### Neutral

- **The gateway is in-process only.** That keeps deployment topology
  simple (one binary embeds the gateway and the adapters it uses) and
  matches how dear-agent is run today. If we ever want a "gateway
  daemon" with adapters in other processes, that's a separate ADR.

---

## Implementation notes

- Files: `pkg/gateway/{gateway,handlers,messages,errors}.go`,
  `pkg/gateway/adapters/cli/cli.go`,
  `pkg/gateway/adapters/http/http.go`, plus `_test.go` companions.
- The HTTP adapter is constructed with an existing `*api.Server` and a
  `*Gateway`; it injects an `api.Runner` whose `Run` builds a Command
  and calls `Dispatch`. No new HTTP routes are added in this ADR.
- The CLI adapter is constructed with `io.Reader` / `io.Writer` and a
  default `Caller`. It's used directly by tests; a thin
  `cmd/dear-agent-gateway-cli` binary is a follow-up.
- `pkg/api/server.go` is **not** modified by this ADR. The HTTP adapter
  wraps it from outside; the existing direct integration keeps working
  for callers that don't want to introduce a gateway.
- The gateway publishes events but no source emits them yet. The
  workflow runner will grow a hook in a follow-up so HITL openings and
  run completions reach `Publish`. Until then, the event channel is
  exercised by tests only.

---

## Alternatives considered

**Build per-surface adapters directly into `pkg/api` and let MCP stay
separate.** Rejected: every new platform doubles the maintenance burden
on `pkg/api`, and the MCP server already proves we want a non-HTTP
surface. A common bus is the simpler endpoint.

**Use a third-party message bus (NATS, Redis Pub/Sub) as the gateway.**
Rejected: external infra for an in-process call site. The whole point
is consolidating dispatch on the existing substrate; adding a broker
inverts the dependency.

**Strongly typed `Command` per type instead of `Args map[string]any`.**
Considered. The tradeoff is compile-time safety (typed) vs. uniform
dispatcher (untyped). We chose untyped because (a) adapter wire formats
are already untyped JSON, (b) the type-specific shape lives in
`pkg/workflow` already, and (c) handler unit tests assert the contract.
A future ADR can introduce typed wrappers if the untyped path produces
real bugs.
