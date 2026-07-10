# Wayfinder Analytics Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Canonical phase and session quality metrics.

## EARS Requirements

**WAYFINDER-ANALYTICS-01** When phase quality is calculated, the system shall reduce score for rework and errors and clamp the result between zero and one.

**WAYFINDER-ANALYTICS-02** When phase metrics are aggregated, the system shall report total errors, total rework, affected canonical phases, and average and overall quality.

**WAYFINDER-ANALYTICS-03** When a session starts, the system shall assign a stable session identifier and publish lifecycle telemetry when an event bus is available.

**WAYFINDER-ANALYTICS-04** When canonical phases start and complete, the system shall record timing, outcome, and metadata under the active session.

**WAYFINDER-ANALYTICS-05** When no event bus is configured, the system shall preserve local session tracking without failing.

## Test Traceability

- Package tests: `wayfinder/internal/analytics/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
