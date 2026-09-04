# SPEC: agm/internal/marketplaceparity/SPEC.md
# RELATED-SPEC: agm/internal/plugin/SPEC.md
# RELATED-SPEC: .claude-plugin/SPEC.md
# RELATED-SPEC: .dear-agent/SPEC.md
Feature: SKILL and plugin marketplace parity
  AGM should publish one marketplace contract that every active harness can
  consume. Claude Code uses the native plugin marketplace, while Codex CLI, AGY,
  OpenCode and Pi use the harness-neutral catalog with AGENTS.md/SKILL fallback.
  The Claude-only SPEC-governance source projection is an explicit exception to
  the shared neutral-to-Claude plugin inventory.

  Scenario Outline: Active harnesses have marketplace discovery surfaces
    Given harness "<harness>" is configured
    When AGM validates marketplace parity
    Then harness "<harness>" should have a marketplace discovery surface
    And the marketplace discovery surface should use the expected mode

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario: Claude marketplace mirrors shared plugins with its approved exception
    When AGM validates marketplace catalog mirrors
    Then the Claude marketplace should match the neutral marketplace catalog

  Scenario Outline: Marketplace plugins publish required assets
    Given marketplace plugin "<plugin>" is configured
    When AGM validates marketplace parity
    Then marketplace plugin "<plugin>" should publish its declared assets

    # SPEC-governance is a Claude-only catalog exception covered by the
    # deterministic projection contract tests, not this shared-catalog outline.
    Examples:
      | plugin    |
      | agm       |
      | wayfinder |
      | youtube   |
      | research-pipeline |
