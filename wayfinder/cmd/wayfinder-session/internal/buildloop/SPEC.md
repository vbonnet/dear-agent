# Wayfinder BUILD Loop Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Canonical BUILD-phase test-first state machine and iteration metrics.

## EARS Requirements

**WAYFINDER-BUILDLOOP-01** When a BUILD loop is created without configuration, the system shall apply bounded retries, test-first enforcement, quality thresholds, and execution timeouts.

**WAYFINDER-BUILDLOOP-02** When a task advances, the system shall allow only declared state transitions and shall reject invalid transitions.

**WAYFINDER-BUILDLOOP-03** When test-first, coding, green, validation, deployment, or monitoring exit criteria are unmet, the system shall keep the task from advancing.

**WAYFINDER-BUILDLOOP-04** When a transition enters an error state, the system shall increment retry state and shall stop retrying at the configured limit.

**WAYFINDER-BUILDLOOP-05** While concurrent callers record state transitions, the system shall preserve race-free transition history and iteration metrics.

**WAYFINDER-BUILDLOOP-06** When a task completes the full canonical BUILD cycle, the system shall return success with duration, state visits, test runs, and review metrics.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/buildloop/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
