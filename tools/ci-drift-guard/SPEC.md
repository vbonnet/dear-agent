# CI Drift Guard Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Detection of drift between workflow job names and protected-branch checks.

## EARS Requirements

**CI-DRIFT-01** When a workflow job name contains matrix placeholders, the system shall expand every configured matrix combination into concrete check names.

**CI-DRIFT-02** When workflow files are inspected, the system shall collect the effective check names for ordinary, matrix, and CodeQL jobs.

**CI-DRIFT-03** When required-check API input has an unexpected shape, the system shall reject the malformed response.

**CI-DRIFT-04** When protected-branch checks and workflow checks differ, the system shall report required checks with no producer and produced checks with no requirement.

**CI-DRIFT-05** When protected-branch checks and workflow checks match, the system shall report a clean drift result.

## Test Traceability

- Package tests: `tools/ci-drift-guard/main_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
