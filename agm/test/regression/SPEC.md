# AGM Regression Tests Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**REGR-01** When a known archive regression is tested, the suite shall preserve attached-session force requirements and stopped-session storage consistency.

**REGR-02** When tmux socket regressions are tested, the suite shall require every tmux operation to target detected socket state without a default-socket fallback.

**REGR-03** When prompt interruption regressions are tested, the suite shall preserve literal command delivery and explicit interrupt ordering.

**REGR-04** When naming regressions are tested, the suite shall reject retired product names in user-facing configuration and documentation surfaces.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Regression tests: `agm/test/regression/*_test.go`
