# SPEC: cmd/agm-worktree-audit/SPEC.md
# RELATED-SPEC: cmd/resolve-review-threads/SPEC.md
# RELATED-SPEC: cmd/pr-blockers/SPEC.md
# RELATED-SPEC: cmd/changed-paths/SPEC.md
Feature: Workflow tooling guardrails
  Agent workflow support commands should have explicit SPEC coverage and BDD
  traceability so cleanup and review-thread tooling cannot drift outside the
  safe local development rules.

  Scenario Outline: Workflow tooling commands declare SPEC coverage
    Given workflow tooling command "<command>" is configured
    When AGM validates workflow tooling command coverage
    Then workflow tooling command "<command>" should have a co-located SPEC

    Examples:
      | command                |
      | agm-worktree-audit     |
      | resolve-review-threads |
      | pr-blockers            |
      | changed-paths          |
