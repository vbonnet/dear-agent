# AGM Examples Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**EXS-01** When the monitoring example checks daemon health, the example shall derive the PID path from the current user home and close the message queue after use.

**EXS-02** If daemon health or queue access fails, then the example shall report the failure and return without dereferencing unavailable state.

**EXS-03** When the metrics example records delivery, detection, queue, and polling events, the example shall render the resulting metrics snapshot and matching alerts.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/examples/*_test.go`
