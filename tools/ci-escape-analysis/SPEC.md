# CI Escape Analysis Requirements Specification (EARS)

<!-- Last audited at: 2026-08-16 -->

**Version**: 1.0
**Status**: Active
**Scope**: Classification of how a failure on main got past pre-merge, and pricing of the fix.

## EARS Requirements

**CI-ESCAPE-01** When a red check on main is produced by a workflow with no pull-request trigger, the system shall classify it as post-merge-only and shall not recommend filter refinement.

**CI-ESCAPE-02** When a red check on main reported no check run on the introducing pull request, the system shall classify it as never-ran and shall identify filter refinement as the lever.

**CI-ESCAPE-03** When a red check on main was skipped by a job condition on the introducing pull request, the system shall classify it as a selection gap and shall identify filter refinement as the lever.

**CI-ESCAPE-04** When a red check on main failed on the introducing pull request, the system shall classify it as a gating gap and shall distinguish required from advisory contexts.

**CI-ESCAPE-05** When a red check on main passed pre-merge at a deliberately narrower scope, the system shall classify it as a scope gap and shall state that the behaviour is by design.

**CI-ESCAPE-06** When no pre-merge runs exist to measure, the system shall report the prevention cost as unmeasured and shall not report the check as free to prevent.

**CI-ESCAPE-07** When prevention cost is measured, the system shall compute `(cure x frequency) / prevention` and shall render the arithmetic alongside the verdict.

**CI-ESCAPE-08** When sweeping, the system shall file one retrospective per red workflow, shall comment on an existing open retrospective rather than opening a duplicate, and shall close retrospectives whose workflow has recovered.

**CI-ESCAPE-09** When invoked in dry-run mode, the system shall render what it would file and shall not create, comment on, or close any issue.

## Test Traceability

- Package tests: `pkg/cihealth/escape_test.go`
- Command tests: `tools/ci-escape-analysis/triggers_test.go`
- Design: `docs/adr/ADR-038-ci-path-scoping-and-gateway.md`

## BDD Traceability

- Feature: `agm/test/bdd/features/ci_health_escape_analysis.feature`
