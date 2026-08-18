# ADR-001: Queue delivery loop

Status: Superseded by [ADR-002](ADR-002-shared-direct-delivery.md)

Date: 2026-07-17

## Context

AGM needs to deliver persistent inter-session messages without injecting input
while a target tmux pane is busy or blocked. Earlier ADRs in this directory
described a different product: a two-second visual session monitor with an HTTP
status API and mirrored status files. That surface was not the current daemon.

## Decision

`agm-daemon` is a local queue worker. It polls the SQLite message queue on the
interval owned by `internal/contracts`, resolves the target, and uses
`CheckSessionDelivery` as the readiness authority. Display-state detection is
diagnostic only.

Successful tmux delivery uses the safe multiline sender, then attempts session,
queue, metric, and acknowledgment bookkeeping. Those post-send writes are
best-effort and failures are logged; a failed queue update can leave an entry
pending for redelivery. Busy or blocked targets remain pending. Resolution or
send failures request a configured retry attempt, but retry bookkeeping is also
best-effort: if the counter or terminal-state write fails, the durable entry can
remain eligible for another attempt.

The daemon exposes operational state through its PID file, log, queue records,
and AGM command surfaces. It does not own a session-status HTTP or status-file
API.

## Consequences

- Delivery latency is bounded by the configured poll interval when a target
  becomes ready.
- The queue remains the durable delivery record across daemon restarts.
- Readiness policy has one owner, reducing divergence between visual labels and
  delivery behavior.
- Adding a network status surface requires a separate decision, threat model,
  implementation, and tests.

## Evidence

- `agm/internal/daemon/daemon.go`
- `agm/internal/contracts/contracts.go`
- `agm/internal/messages/queue.go`
- `agm/internal/session/state_detector.go`
