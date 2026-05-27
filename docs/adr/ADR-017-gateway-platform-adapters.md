# ADR-017: Gateway and Platform Adapters

**Status**: Proposed (2026-05-03)

Three user-facing surfaces — workflow CLIs, `cmd/dear-agent-mcp`, and
the Tailscale API ([ADR-013](ADR-013-tailscale-api.md)) — each
re-implement the same wiring: parse a request, look up the caller,
dispatch into the engine, marshal a response. `pkg/api` and the MCP
server validate `RunRequest.File` for shell metacharacters with subtly
different rules. A Discord bot adapter would be the third copy. The HTTP
API is also request/response only: when a HITL gate opens during a run,
the only way to learn about it is polling.

Introduce `pkg/gateway/` — an in-process, transport-agnostic message
bus. Three message types (`Command` / `Response` / `Event`), one
dispatcher, one `Adapter` interface, two built-in adapters (CLI, HTTP).

- **Dispatch by command type.** Handlers are wrapped over `pkg/workflow`
  (`run`, `status`, `list`, `logs`, `gates`, `approve`, `reject`,
  `cancel`). The handler set is a struct of fields, not a global; tests
  construct their own gateway with stubs.
- **`Args` / `Body` / `Payload` are `map[string]any`.** The dispatcher
  routes to handlers that already own canonical typed shapes
  (`workflow.RunStatus`, `workflow.HITLRequest`). Compile-time typing
  buys little when the wire format is JSON; a small set of centralised
  arg-key constants per command type catches typos.
- **The gateway publishes events but persists nothing.** `runs.db` is
  already the event log. Adapters that need durability subscribe and
  write to their own store.
- **In-process only.** A multi-process topology with adapters in other
  processes is its own ADR.
- **Caller identity flows from adapter to handler unchanged.** The
  gateway never invents a caller. CLI adapter defaults to `os/user`;
  HTTP adapter delegates to the existing `api.Identifier` (Tailscale
  `WhoIs`); future Discord adapter maps the Discord user id via a static
  YAML mapping.

The HTTP adapter wraps the existing `*api.Server` and routes inbound
traffic through `Dispatch`. **No new HTTP routes** — the point is to
consolidate dispatch policy (rate limits, tracing, future caller-based
authorization) in one place. The CLI adapter reads JSON envelopes from
`io.Reader` / `io.Writer`; useful for tests, scripts, and a future
`cmd/dear-agent-gateway-cli` binary.

### Alternatives rejected

- **Per-surface adapters directly in `pkg/api`, MCP stays separate.**
  Every new platform doubles `pkg/api`'s maintenance burden, and the MCP
  server already proves we want a non-HTTP surface.
- **Third-party message bus (NATS, Redis Pub/Sub).** External infra for
  an in-process call site. The point is consolidating dispatch on the
  existing substrate; adding a broker inverts the dependency.
- **Strongly-typed `Command` per type.** Considered. We chose untyped
  because (a) adapter wire formats are already untyped JSON, (b) typed
  shape lives in `pkg/workflow`, (c) handler unit tests assert the
  contract. A future ADR can add typed wrappers if real bugs emerge.

Discord, Slack, Matrix adapters are defined as the next consumers but
not built here; each gets its own credentials story. MCP-server-as-an-
adapter is plausible later, after chat adapters prove the abstraction.
The cost of the indirection (handler → adapter shim → `Dispatch` →
handler) is two function calls and one map allocation per request — invisible at HTTP rates, conceptual ("where do I look to debug a
failing /run?") only.
