# AGM Lifecycle Telemetry Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/lifecycle` emits OpenTelemetry spans for terminal session lifecycle
transitions. The package keeps session lifecycle observability harness-neutral:
Claude Code, Codex CLI, AGY, OpenCode, and Pi can all update state through different
surfaces while producing the same terminal lifecycle span contract.

## EARS Requirements

**LIFECYCLE-01** When a session transitions to `STOPPED`, the system shall emit one `session.lifecycle` span.

**LIFECYCLE-02** When a session transitions to `OFFLINE`, the system shall emit one `session.lifecycle` span.

**LIFECYCLE-03** When a session lifecycle span is emitted, the system shall attach `session.name`, `session.state`, and `session.state.source` attributes.

**LIFECYCLE-04** When a session lifecycle span is emitted, the system shall mark the span status as OK.

**LIFECYCLE-05** When a session transitions to a non-terminal state, the system shall not emit a lifecycle span.

## BDD Traceability

- `agm/test/bdd/features/harness_parity.feature`

## Package Test Traceability

- `agm/internal/lifecycle/span_test.go`
