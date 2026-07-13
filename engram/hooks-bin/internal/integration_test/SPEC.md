# Engram Hook Internal Integration Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**EHII-01** When internal hook scenarios replay the maintained incident classes, the suite shall verify each detector produces the expected intervention outcome.

**EHII-02** When internal hook state is resumed, the suite shall restore relevant context without relying on a specific agent harness.

**EHII-03** If an incident precondition is absent, then the suite shall verify the hook remains quiet rather than producing a false intervention.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Integration tests: `engram/hooks-bin/internal/integration_test/*_test.go`
