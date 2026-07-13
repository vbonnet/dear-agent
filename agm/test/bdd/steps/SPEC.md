# AGM BDD Step Definitions Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**BDDS-01** When a BDD step group is registered, the package shall initialize scenario-local state rather than sharing mutable state across scenarios.

**BDDS-02** When a package guardrail validates a SPEC, the step shall require a co-located file and an explicit reference to the executing feature.

**BDDS-03** If scenario state is absent or incomplete, then the step shall return an explicit initialization error.

**BDDS-04** When harness and model parity is declared, the step definitions shall validate every active harness against every supported model family.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/test/bdd/steps/*_test.go`
