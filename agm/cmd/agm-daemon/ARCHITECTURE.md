# AGM daemon architecture

<!-- Last audited at: 2026-07-17 -->

`agm-daemon` is the background delivery worker for AGM's persistent message
queue. It waits until a target tmux session can safely receive input, delivers
the queued message, and records queue and acknowledgment state.

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
    -> resolve target session
    -> inspect pane state for diagnostics
    -> CheckSessionDelivery (delivery authority)
         yes       -> send through tmux, update state, mark delivered, ack
         busy/no   -> leave pending for a later tick
         not found -> increment retry or mark permanently failed
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
| Session resolution and state updates | `agm/internal/session` |
| Delivery readiness | `agm/internal/session.CheckSessionDelivery` |
| Safe multiline tmux delivery | `agm/internal/tmux.SendMultiLinePromptSafe` |
| Runtime thresholds | `agm/internal/contracts` |

## Storage boundaries

The message queue is SQLite with WAL enabled at
`~/.config/agm/message_queue.db`. It owns pending, delivered, failed, retry, and
acknowledgment fields. Session metadata is a separate concern owned by
`internal/dolt` and session manifests.

The daemon attempts to create a Dolt adapter at startup. If Dolt is unavailable,
session resolution can fall back to manifest-based lookup. Queue availability is
not optional: failure to open the message queue prevents startup.

## Delivery authority

`session.DetectState` produces display state for logging and metrics. A failed or
unknown display-state read does not itself block delivery.

`session.CheckSessionDelivery` is the sole readiness authority. It distinguishes:

- a pane ready to receive input;
- a busy pane that should remain queued;
- a blocking prompt that should remain queued;
- a missing tmux session that should consume a retry attempt.

This separation prevents a cosmetic state detector from becoming a second,
conflicting delivery policy.

## Retry semantics

Resolution and send failures increment the queue attempt count. Reaching the
configured maximum marks the message permanently failed. The exponential
backoff value is currently diagnostic; retry execution occurs on a subsequent
poll tick rather than through a per-message timer.

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
