# SPEC: agm/internal/engramparity/SPEC.md
Feature: Engram harness parity
  AGM should preserve Engram retrieval metadata and context injection contracts
  across every active harness.

  Scenario Outline: Active harnesses have Engram integration surfaces
    Given harness "<harness>" is configured
    When AGM validates Engram parity
    Then harness "<harness>" should have an Engram injection surface
    And harness "<harness>" should persist Engram metadata through the shared manifest

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |

  Scenario: Engram metadata is harness-neutral
    When AGM validates Engram metadata parity
    Then Engram metadata should be stored in harness-neutral fields
