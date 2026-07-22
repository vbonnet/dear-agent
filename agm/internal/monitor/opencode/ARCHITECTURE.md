# OpenCode monitor architecture

<!-- Last audited at: 2026-07-18 -->

`agm/internal/monitor/opencode` adapts OpenCode's HTTP server-sent event stream
to AGM's harness-neutral session-state events. The package is an optional
monitoring adapter; it does not launch OpenCode or own AGM session persistence.

## Boundary and flow

```text
OpenCode GET /event
  -> SSEAdapter reads data and heartbeat lines
  -> EventParser validates and maps event types
  -> Publisher creates EventSessionStateChange
  -> eventbus.EventBus broadcasts best effort
```

The top-level `Adapter` owns component construction, a startup health probe,
SSE start/stop, and an in-memory OpenCode-to-AGM session ID map.

## Components

- `SSEAdapter` owns the HTTP streaming connection, heartbeat/event timestamps,
  reconnect loop, connection health, and shutdown synchronization.
- `EventParser` validates JSON events and maps OpenCode event types to AGM
  states.
- `Publisher` adds the AGM session ID, monotonic sequence, source, harness, and
  metadata before broadcasting an EventBus event.
- `Adapter` wires those parts and performs the configured HTTP health probe.
- `SessionMapper` is a mutex-protected, process-local identifier map.

## State mapping

| OpenCode event | AGM state |
|---|---|
| `permission.asked` | `AWAITING_PERMISSION` |
| `tool.execute.before` | `WORKING` |
| `tool.execute.after` | `IDLE` |
| `session.created` | `DONE` |
| `session.closed` | `TERMINATED` |
| unknown type | `WORKING` |

Unknown events fail safe as working, never idle. Permission, tool, session, and
original event-type fields are retained when present.

## Connection and lifecycle semantics

`NewSSEAdapter` configures a streaming HTTP client with dial, TLS handshake,
response-header, and idle-connection timeouts, plus a 30-second TCP keep-alive
probe interval. The keep-alive value is not a 30-second liveness deadline; an
otherwise healthy stream with no application heartbeat is not disconnected on
that schedule. `Start` attempts
`GET <ServerURL>/event`; a failed first connection returns an error and starts
the reconnect loop. Reconnect delay grows from `InitialDelay` by `Multiplier`
and is capped at `MaxDelay`. A positive `MaxRetries` stops further connection
attempts after the failure counter reaches that value.

`Stop` cancels the lifecycle context, closes the active response body, and
waits for work currently registered in the adapter wait group until the
caller's context expires. A reconnect loop entered after a reader unregisters
is cancellation-aware but is not covered by that wait guarantee.
Health reports connection state plus the last event and heartbeat. After the
first heartbeat, more than five minutes without another reports an error. A
connected stream that has never emitted a heartbeat remains healthy based on
connection state alone.

The top-level startup probe is not a tmux failover implementation. A failed
probe returns an error and leaves the adapter inactive.

## Delivery and observability constraints

EventBus broadcast is non-blocking and best effort. `PublishWithBackpressure`
retries errors raised while constructing the local EventBus event, but the
`Broadcast` interface does not report queue acceptance. After repeated local
publication failures, the publisher asks the adapter to stop asynchronously.

`incrementMetric` currently writes structured diagnostic log entries. There is
no Prometheus exporter, durable metric counter, or verified performance claim
in this package. Add documentation for those only with an implemented surface
and a dated reproducible verification command.

## Extension constraints

- Add event mappings in `EventParser` and cover their safe-state semantics.
- Preserve monotonic publisher sequence values and the `opencode-sse` source.
- Do not infer permission approval from a state event.
- Add an explicit monitor-selection owner before introducing any fallback mode.
- If EventBus delivery acknowledgement becomes required, deepen the publisher
  interface instead of presenting best-effort broadcast as a queue guarantee.

## Verification

```sh
go test -race ./agm/internal/monitor/opencode -count=1
```

The package tests cover parsing, connection/reconnect behavior, lifecycle,
publishing, sequence ordering, and concurrent session mapping.
