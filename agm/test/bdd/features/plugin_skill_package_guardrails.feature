# SPEC: pkg/plugin/SPEC.md
# RELATED-SPEC: engram/internal/plugin/SPEC.md
# RELATED-SPEC: pkg/skilllint/SPEC.md
# RELATED-SPEC: tools/skill-lint/SPEC.md
Feature: Plugin and skill package guardrails
  Plugin, marketplace, and skill governance packages should carry executable
  SPEC traceability because marketplace parity depends on stable manifest,
  loader, execution, and cost-lint contracts.

  Scenario Outline: Plugin and skill packages declare SPEC coverage
    Given plugin and skill package "<package>" is configured
    When AGM validates plugin and skill package coverage
    Then plugin and skill package "<package>" should have a co-located SPEC

    Examples:
      | package                |
      | engram/internal/plugin |
      | pkg/plugin             |
      | pkg/skilllint          |
      | tools/skill-lint       |
