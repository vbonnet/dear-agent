# Wayfinder Project Discovery Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Safe project directory, workspace, identifier, and AGM session discovery.

## EARS Requirements

**WAYFINDER-PROJECT-01** When a project directory is requested, the system shall resolve an explicit path or detect the current project directory.

**WAYFINDER-PROJECT-02** When a path is validated against a workspace, the system shall reject paths outside the workspace boundary.

**WAYFINDER-PROJECT-03** When a project identifier is generated, the system shall produce a stable filesystem-safe identifier from the prompt.

**WAYFINDER-PROJECT-04** When workspace determination runs, the system shall prefer valid explicit context and reject malformed workspace names.

**WAYFINDER-PROJECT-05** When AGM workspace discovery is available, the system shall parse its output without executing a shell-interpolated command.

**WAYFINDER-PROJECT-06** When session output contains an AGM identifier, the system shall extract the identifier without retaining unrelated output.

## Test Traceability

- Package tests: `wayfinder/internal/project/detect_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
