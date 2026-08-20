# AGM BDD Step Definitions Specification

<!-- Last audited at: 2026-07-27 -->

## Requirements

**BDDS-01** When a BDD step group is registered, the package shall initialize scenario-local state rather than sharing mutable state across scenarios.

**BDDS-02** When a package guardrail validates a SPEC, the step shall require a co-located file and an explicit reference to the executing feature.

**BDDS-03** If scenario state is absent or incomplete, then the step shall return an explicit initialization error.

**BDDS-04** When harness and model parity is declared, the step definitions shall validate every active harness against every supported model family.

**BDDS-05** When sandbox isolation is validated in a shared repository, the step shall compare worktree records within the invoking checkout and shall ignore sibling worktree churn owned by concurrent tasks.

**BDDS-06** When a real-tmux regression self-skips because the execution environment denies process-table inspection, the step definitions shall accept that exact capability diagnosis while rejecting every other unconfigured skip.

**BDDS-07** When a BDD scenario enforces a portability contract also covered by a package regression, the step definition shall call the same canonical checker.

**BDDS-08** When multiple scenarios assert the same immutable behavior by launching an expensive static subprocess test suite, the step definitions shall execute that suite once per BDD process under a bounded contention-tolerant deadline, share the immutable result safely with concurrent callers, and copy the result into scenario-local state without weakening any assertion.

**BDDS-ROOT-01** When BDD steps resolve the repository from a nested package working directory, the system shall find the nearest ancestor containing `go.mod` and `agm` without relying on compiler source paths.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Feature: `agm/test/bdd/features/bdd_root_portability.feature`
- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Package tests: `agm/test/bdd/steps/*_test.go`
