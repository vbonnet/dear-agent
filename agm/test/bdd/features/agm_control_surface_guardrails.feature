# SPEC: agm/internal/api/SPEC.md
# RELATED-SPEC: agm/internal/cli/SPEC.md
# RELATED-SPEC: agm/internal/delegation/SPEC.md
# RELATED-SPEC: agm/internal/discovery/SPEC.md
# RELATED-SPEC: agm/internal/surface/SPEC.md
# RELATED-SPEC: agm/internal/shellquote/SPEC.md
# RELATED-SPEC: agm/internal/terminal/SPEC.md
# RELATED-SPEC: agm/internal/validate/SPEC.md
# RELATED-SPEC: agm/cmd/agm/SPEC.md
Feature: AGM control surface guardrails
  AGM control-plane packages should carry executable SPEC traceability so CLI,
  MCP, tmux, and validation behavior do not drift across harnesses.

  Scenario Outline: AGM control surface packages declare SPEC coverage
    Given AGM control surface package "<package>" is configured
    When AGM validates control surface package coverage
    Then AGM control surface package "<package>" should have a co-located SPEC

    Examples:
      | package                 |
      | agm/internal/api        |
      | agm/internal/cli        |
      | agm/internal/delegation |
      | agm/internal/discovery  |
      | agm/internal/shellquote |
      | agm/internal/surface    |
      | agm/internal/terminal   |
      | agm/internal/validate   |

  Scenario: Cobra command validation tests are order independent
    Given AGM Cobra commands with mutable execution state
    When AGM audits Cobra command test isolation
    Then mutable command flags should belong to fresh command instances
    And command validation tests should exercise repeatable execution orders
