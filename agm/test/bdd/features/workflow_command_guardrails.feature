# SPEC: cmd/workflow-run/SPEC.md
# RELATED-SPEC: cmd/workflow-approve/SPEC.md
# RELATED-SPEC: cmd/workflow-audit/SPEC.md
# RELATED-SPEC: cmd/workflow-cancel/SPEC.md
# RELATED-SPEC: cmd/workflow-codemod/SPEC.md
# RELATED-SPEC: cmd/workflow-dev/SPEC.md
# RELATED-SPEC: cmd/workflow-inspector/SPEC.md
# RELATED-SPEC: cmd/workflow-lint/SPEC.md
# RELATED-SPEC: cmd/workflow-list/SPEC.md
# RELATED-SPEC: cmd/workflow-logs/SPEC.md
# RELATED-SPEC: cmd/workflow-migrate/SPEC.md
# RELATED-SPEC: cmd/workflow-roles/SPEC.md
# RELATED-SPEC: cmd/workflow-status/SPEC.md
Feature: Workflow command guardrails
  Workflow CLI packages should carry executable SPEC traceability because they
  are the operator-facing control surface for workflow persistence, HITL,
  audit, migration, and development loops.

  Scenario Outline: Workflow command packages declare SPEC coverage
    Given workflow command package "<command>" is configured
    When AGM validates workflow command package coverage
    Then workflow command package "<command>" should have a co-located SPEC

    Examples:
      | command                |
      | cmd/workflow-approve   |
      | cmd/workflow-audit     |
      | cmd/workflow-cancel    |
      | cmd/workflow-codemod   |
      | cmd/workflow-dev       |
      | cmd/workflow-inspector |
      | cmd/workflow-lint      |
      | cmd/workflow-list      |
      | cmd/workflow-logs      |
      | cmd/workflow-migrate   |
      | cmd/workflow-roles     |
      | cmd/workflow-run       |
      | cmd/workflow-status    |

  Scenario Outline: Workflow run specification uses canonical long flags
    Given workflow command package "cmd/workflow-run" is configured
    When AGM validates workflow command package coverage
    Then workflow command package SPEC should declare requirement "<requirement>" containing "<flag>"

    Examples:
      | requirement     | flag          |
      | WORKFLOW-RUN-01 | --file        |
      | WORKFLOW-RUN-02 | --dry-run     |
      | WORKFLOW-RUN-03 | --input       |
      | WORKFLOW-RUN-05 | --db          |
      | WORKFLOW-RUN-07 | --roles       |
      | WORKFLOW-RUN-08 | --trigger     |
