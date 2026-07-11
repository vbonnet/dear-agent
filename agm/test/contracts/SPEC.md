# AGM Adapter Pact Tests Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**PACT-01** When an adapter pact is evaluated, the suite shall verify the shared adapter contract independently of provider-specific implementation details.

**PACT-02** When provider payloads differ, the pact suite shall preserve equivalent session, prompt, and response outcomes.

**PACT-03** If a pact fixture violates the adapter contract, then the suite shall fail before the incompatible adapter reaches integration tests.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Pact tests: `agm/test/contracts/*_test.go`
