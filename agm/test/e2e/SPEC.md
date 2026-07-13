# AGM End-to-End Tests Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**E2E-01** When the end-to-end suite starts, the suite shall build or locate the AGM binary and isolate its filesystem and process state.

**E2E-02** When worker lifecycle behavior is exercised, the suite shall verify creation, observation, messaging, and cleanup through user-facing commands.

**E2E-03** When status-line behavior is exercised, the suite shall verify plain and JSON output, context thresholds, git state, and cache behavior.

**E2E-04** If an external harness prerequisite is unavailable, then the suite shall skip only the dependent scenario with an explicit reason.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- End-to-end tests: `agm/test/e2e/*_test.go`
