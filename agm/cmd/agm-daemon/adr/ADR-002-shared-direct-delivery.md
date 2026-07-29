# ADR-002: Shared direct-delivery transaction

Status: Accepted

Date: 2026-07-27

## Context

ADR-001 made `session.CheckSessionDelivery` the daemon readiness authority and
paired it with a separate tmux sender. Since then, `internal/ops.SendMessage`
has become a deep operation that owns stable-recipient resolution, API/tmux
routing, lifecycle checks, and atomic tmux readiness plus exact-pane send.

Keeping the daemon sandwich beside that operation creates two authorities. The
CLI had a similar preliminary classifier, while MCP already used the operation
directly.

## Decision

CLI, MCP, and daemon direct sends use `internal/ops.SendMessage`.

The operation owns:

- storage and manifest resolution;
- stable session identity;
- API versus tmux selection;
- lifecycle and harness readiness;
- atomic exact-pane tmux delivery or provider delivery;
- typed delivered or not-ready outcomes.

The daemon owns:

- queue polling and ordering;
- not-ready defer policy;
- failure retry accounting;
- delivered-state and acknowledgment bookkeeping;
- operational logging and metrics.

A not-ready state other than `NOT_FOUND` leaves the queue entry pending without
consuming a retry. `NOT_FOUND` and other operation failures consume the existing
retry policy. A successful result carries the stable session ID used for
post-delivery state bookkeeping.

Display-state detection remains best-effort diagnostics after the operation; it
does not gate delivery.

## Consequences

- Direct readiness and send are one transaction across all production callers.
- New readiness states are produced once and translated by caller policy.
- The daemon no longer owns a tmux sender or calls
  `session.CheckSessionDelivery`.
- Queue durability, retry limits, poll intervals, PID lifecycle, and
  acknowledgment behavior do not move into the operation.
- Provider and tmux delivery share the same recipient-shaped interface without
  forcing queue scheduling into that interface.

## Evidence

- `agm/internal/ops/session_send.go`
- `agm/cmd/agm/send_msg.go`
- `agm/internal/daemon/daemon.go`
- `agm/test/bdd/steps/harness_spec_guardrails_steps.go`
