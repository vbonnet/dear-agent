# AGM daemon specification

<!-- Last audited at: 2026-07-17 -->

## Executable EARS requirements

**AGMD-01** When the daemon polls a queued message, the system shall use `CheckSessionDelivery` as the sole authority for immediate delivery.

**AGMD-02** When the target session is busy or blocked, the system shall leave the message pending without consuming a retry attempt.

**AGMD-03** When session resolution or tmux delivery fails, the system shall increment the attempt count or mark the message permanently failed at the configured limit.

**AGMD-04** When delivery succeeds, the system shall update session state, mark the queue entry delivered, and record acknowledgment state.

**AGMD-05** When the daemon receives a supported shutdown signal, the system shall stop polling and remove its owned PID file.

**AGMD-06** The system shall not expose a daemon session-status HTTP API or status-file interface unless a new accepted decision and implementation add that surface.

## BDD traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

## Executable owners

- `agm/cmd/agm-daemon/main.go`
- `agm/internal/daemon/daemon.go`
- `agm/internal/messages`
- `agm/internal/contracts`
