# AGM Daemon Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/daemon` owns background queue delivery, state polling, OpenCode SSE
adapter startup, fallback behavior, PID-file exclusivity, retry handling, health,
and daemon metrics. The daemon must preserve the shared message-delivery contract
across harnesses so non-Claude harnesses do not depend on Claude-specific hooks
to receive queued work.

## EARS Requirements

**DAEMON-01** When the daemon starts, the system shall write a PID file and refuse to start a second daemon for the same configured PID path.

**DAEMON-02** When the daemon exits, the system shall remove the PID file it created.

**DAEMON-03** When the daemon receives SIGINT, SIGTERM, or SIGHUP, the system shall cancel its run loop and shut down gracefully.

**DAEMON-04** When OpenCode adapter support is enabled and an event bus is configured, the system shall initialize the OpenCode SSE adapter from shared configuration.

**DAEMON-05** When OpenCode adapter startup fails and fallback is enabled, the system shall continue using tmux monitoring for OpenCode sessions.

**DAEMON-06** When OpenCode adapter startup fails and fallback is disabled, the system shall report that OpenCode sessions will not be monitored until the adapter succeeds.

**DAEMON-07** When the daemon starts, the system shall retry recently failed messages before entering the periodic poll loop.

**DAEMON-08** When a poll tick runs, the system shall update queue-depth, poll-duration, delivery-attempt, state-detection, and alert metrics from the same collector.

**DAEMON-09** When a queued message targets a session that is not ready, the system shall defer delivery instead of losing or corrupting the message.

**DAEMON-10** When a queued message is delivered successfully, the system shall acknowledge it when an acknowledgment manager is configured.

**DAEMON-11** When health status is requested, the system shall report daemon running state, queue depth, recent poll timing, and metric health from current daemon state.

## BDD Traceability

- `agm/test/bdd/features/harness_parity.feature`
- `agm/test/bdd/features/stall_detection.feature`

## Package Test Traceability

- `agm/internal/daemon/daemon_test.go`
- `agm/internal/daemon/metrics_test.go`
- `agm/internal/daemon/adapter_integration_test.go`
