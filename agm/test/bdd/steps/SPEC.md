# AGM BDD Step Definitions Specification

<!-- Last audited at: 2026-07-11 -->

## Requirements

**BDDS-01** When a BDD step group is registered, the package shall initialize scenario-local state rather than sharing mutable state across scenarios.

**BDDS-02** When a package guardrail validates a SPEC, the step shall require a co-located file and an explicit reference to the executing feature.

**BDDS-03** If scenario state is absent or incomplete, then the step shall return an explicit initialization error.

**BDDS-04** When harness and model parity is declared, the step definitions shall validate every active harness against every supported model family.

**BDDS-05** When sandbox isolation is validated in a shared repository, the step shall compare worktree records within the invoking checkout and shall ignore sibling worktree churn owned by concurrent tasks.

**BDDS-ROOT-01** When BDD steps resolve the repository from a nested package working directory, the system shall find the nearest ancestor containing `go.mod` and `agm` without relying on compiler source paths.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Feature: `agm/test/bdd/features/bdd_root_portability.feature`
- Package tests: `agm/test/bdd/steps/*_test.go`
