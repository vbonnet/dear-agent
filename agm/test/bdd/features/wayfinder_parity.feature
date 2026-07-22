# SPEC: agm/internal/wayfinderparity/SPEC.md
# RELATED-SPEC: agm/internal/a2a/channel/SPEC.md
# RELATED-SPEC: agm/internal/a2a/config/SPEC.md
# RELATED-SPEC: agm/internal/a2a/wayfinder/SPEC.md
# RELATED-SPEC: .pi/SPEC.md
Feature: Wayfinder harness parity
  AGM should expose Wayfinder workflow discovery, execution, and status surfaces
  across every active harness.

  Scenario Outline: Active harnesses have Wayfinder discovery surfaces
    Given harness "<harness>" is configured
    When AGM validates Wayfinder parity
    Then harness "<harness>" should have a Wayfinder discovery surface
    And harness "<harness>" should have a Wayfinder execution surface

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario: Wayfinder assets and MCP operations are published
    When AGM validates Wayfinder asset parity
    Then Wayfinder should publish SKILL, plugin, CLI, and MCP status surfaces

  Scenario: Wayfinder phase guidance uses harness-neutral Engrams
    When AGM validates Wayfinder phase Engram parity
    Then Wayfinder should resolve phase Engrams without harness-specific state
