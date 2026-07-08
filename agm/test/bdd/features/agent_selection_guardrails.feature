# SPEC: agm/internal/agents/SPEC.md
Feature: Agent selection guardrails
  AGM's legacy AGENTS.md keyword-routing package should carry executable SPEC
  traceability so harness selection compatibility remains explicit while
  parity registries evolve.

  Scenario Outline: Agent selection packages declare SPEC coverage
    Given agent selection package "<package>" is configured
    When AGM validates agent selection package coverage
    Then agent selection package "<package>" should have a co-located SPEC

    Examples:
      | package             |
      | agm/internal/agents |

