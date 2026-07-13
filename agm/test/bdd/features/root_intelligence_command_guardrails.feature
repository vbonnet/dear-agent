# SPEC: cmd/dear-agent-search/SPEC.md
# RELATED-SPEC: cmd/backlog-suggest/SPEC.md
# RELATED-SPEC: cmd/code-intel/SPEC.md
# RELATED-SPEC: cmd/dear-agent-signals/SPEC.md
# RELATED-SPEC: cmd/eval-extract/SPEC.md
Feature: Root intelligence command guardrails
  Deterministic repository intelligence commands should keep executable SPEC
  traceability and return the same shared data across harnesses and models.

  Scenario Outline: Intelligence command packages declare SPEC coverage
    Given intelligence command package "<package>" is configured
    When AGM validates intelligence command package coverage
    Then intelligence command package "<package>" should have a co-located SPEC

    Examples:
      | package                |
      | cmd/backlog-suggest    |
      | cmd/code-intel         |
      | cmd/dear-agent-search  |
      | cmd/dear-agent-signals |
      | cmd/eval-extract       |
