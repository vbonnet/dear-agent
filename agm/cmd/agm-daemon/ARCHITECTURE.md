# AGM daemon architecture

<!-- Last audited at: 2026-07-27 -->

`agm-daemon` is the background delivery worker for AGM's persistent message
queue. It asks the shared direct-delivery operation to resolve and deliver each
queued message, then records queue and acknowledgment state.

It is not AGM's general session monitor or an HTTP status service.

## Runtime flow

```text
agm-daemon main
    -> open ~/.config/agm/message_queue.db
    -> optionally connect to Dolt for session resolution
    -> start optional Sentinel monitor
    -> start daemon loop

daemon tick (default 30 seconds)
    -> load pending queue entries
    -> internal/ops.SendMessage (direct-delivery transaction)
         resolve stable recipient
         select API or tmux transport
         classify readiness and send atomically
    -> translate typed outcome
         delivered -> update state by stable ID, mark delivered, ack
         not ready -> leave pending for a later tick
         not found/other failure -> increment retry or mark permanently failed
    -> inspect display state for diagnostics only
    -> check acknowledgment timeouts
    -> update metrics and emit alert logs
```

The poll interval, maximum retries, initial backoff, metric history size, and
alert thresholds come from `agm/internal/contracts`. The embedded default poll
interval is 30 seconds; `~/.agm/slo-contracts.yaml` may override it.

## Source owners

| Concern | Executable owner |
|---|---|
| Process wiring, log file, queue, Dolt, Sentinel | `main.go` |
| Signal, PID, poll, delivery, retry lifecycle | `agm/internal/daemon/daemon.go` |
| Queue persistence and ordering | `agm/internal/messages/queue.go` |
| Acknowledgment state | `agm/internal/messages/ack.go` |
| Direct recipient resolution, readiness, and delivery | `agm/internal/ops.SendMessage` |
| Post-delivery display and state updates | `agm/internal/session` |
| Atomic exact-pane tmux input | `agm/internal/tmux.CheckExpectedHarnessInputAndSend` |
| Runtime thresholds | `agm/internal/contracts` |

## Storage boundaries

The message queue is SQLite with WAL enabled at
`~/.config/agm/message_queue.db`. It owns pending, delivered, failed, retry, and
acknowledgment fields. Session metadata is a separate concern owned by
`internal/dolt` and session manifests.

The daemon attempts to create a Dolt adapter at startup. If Dolt is unavailable,
the shared operation fails closed and the queue entry follows retry policy.
Queue availability is not optional: failure to open the message queue prevents
startup.

## Delivery authority

`internal/ops.SendMessage` is the sole direct-delivery authority for the daemon,
CLI, and MCP. It resolves the stable recipient, routes pure API sessions through
their provider transaction, and couples tmux harness readiness to exact-pane
send under the stable-session lock.

The daemon translates typed results without reclassifying readiness:

- delivered results update state by returned stable ID, mark delivered, and ack;
- not-ready results other than `NOT_FOUND` remain queued without a retry;
- `NOT_FOUND` and other operation failures enter retry policy.

`session.DetectState` runs only for best-effort logging and metrics after the
operation. A failed or unknown display-state read does not alter the delivery
outcome.

## Retry semantics

`NOT_FOUND`, resolution, provider, and tmux failures increment the queue attempt
count. Reaching the configured maximum marks the message permanently failed.
The exponential backoff value is currently diagnostic; retry execution occurs
on a subsequent poll tick rather than through a per-message timer.

Messages deferred because the target is busy or blocked do not consume a retry.
At startup, messages that failed within the preceding day are reset for retry by
the queue implementation.

## Process lifecycle

- A PID file at `~/.agm/daemon.pid` prevents concurrent daemon instances.
- Logs are appended to `~/.agm/logs/daemon/daemon.log`.
- `SIGINT`, `SIGTERM`, and `SIGHUP` stop the loop.
- Shutdown cancels optional adapters and removes the owned PID file.
- Sentinel, when configured, runs as a separate monitor goroutine and is stopped
  after the delivery daemon exits.

## Verification

Focused tests live in `agm/internal/daemon`, `agm/internal/messages`, and
`agm/cmd/agm-daemon`. The strict requirements are in [`SPEC.md`](SPEC.md), and
the durable queue-worker decision is in [`adr/README.md`](adr/README.md).
