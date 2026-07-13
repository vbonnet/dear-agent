# Wayfinder Session Integration Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**WFIT-01** When the Wayfinder V2 integration workflow runs, the suite shall exercise the canonical nine-phase lifecycle and persist valid V2 status.

**WFIT-02** When phase transitions are requested, the suite shall verify ordering, completion gates, stakeholder approval, and risk-adaptive review behavior.

**WFIT-03** When the BUILD loop is exercised, the suite shall verify feature generation, test evidence, and completion outcomes.

**WFIT-04** If the host CI environment cannot support the integration prerequisites, then the suite shall skip explicitly without weakening portable V2 unit coverage.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Integration tests: `wayfinder/cmd/wayfinder-session/internal/integration/*_test.go`
