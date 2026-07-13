# AGM Integration Tests Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**INTG-01** When integration tests use tmux or filesystem state, the suite shall isolate those dependencies by socket, directory, workspace, and session identifier.

**INTG-02** When active harness parity is exercised, the suite shall preserve equivalent lifecycle, messaging, data exchange, and session-management outcomes.

**INTG-03** If an integration prerequisite is unavailable in CI, then the suite shall skip only the affected host-dependent test and retain portable assertions.

**INTG-04** When integration resources are created, the suite shall register cleanup that removes processes, sessions, and temporary artifacts.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Integration tests: `agm/test/integration/*_test.go`
