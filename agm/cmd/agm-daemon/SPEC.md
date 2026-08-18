# AGM daemon specification

<!-- Last audited at: 2026-07-27 -->

## Executable EARS requirements

**AGMD-01** When the daemon polls a queued message, the system shall invoke `internal/ops.SendMessage` as the sole direct-delivery transaction.

**AGMD-02** When the shared operation returns a typed not-ready state other than `NOT_FOUND`, the system shall leave the message pending without consuming a retry attempt.

**AGMD-03** When the shared operation returns `NOT_FOUND` or another resolution or delivery failure, the system shall attempt to increment the attempt count or mark the message permanently failed at the configured limit; if that bookkeeping write fails, the system shall log the failure and leave the queue entry pending with its last durable attempt count.

**AGMD-04** When shared direct delivery succeeds, the system shall use the returned stable session ID to attempt a session-state update, mark the queue entry delivered, and record acknowledgment state; bookkeeping failures shall be logged without reporting the already-delivered message as failed.

**AGMD-05** When the daemon receives a supported shutdown signal, the system shall stop polling and remove its owned PID file.

**AGMD-06** The system shall not expose a daemon session-status HTTP API or status-file interface unless a new accepted decision and implementation add that surface.

**AGMD-07** The daemon shall not call `session.CheckSessionDelivery` or own a separate tmux sender.

## BDD traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

## Executable owners

- `agm/cmd/agm-daemon/main.go`
- `agm/internal/daemon/daemon.go`
- `agm/internal/messages`
- `agm/internal/contracts`
