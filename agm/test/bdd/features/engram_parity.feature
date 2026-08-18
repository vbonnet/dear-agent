# SPEC: agm/internal/engramparity/SPEC.md
# RELATED-SPEC: engram/cmd/engram-mcp/SPEC.md
# RELATED-SPEC: engram/internal/beadstore/SPEC.md
# RELATED-SPEC: agm/internal/engram/SPEC.md
# RELATED-SPEC: agm/internal/a2a/beads/SPEC.md
# RELATED-SPEC: agm/internal/a2a/personas/SPEC.md
# RELATED-SPEC: engram/hippocampus/SPEC.md
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
      | pi-cli       |

  Scenario: Engram metadata is harness-neutral
    When AGM validates Engram metadata parity
    Then Engram metadata should be stored in harness-neutral fields

  Scenario Outline: Hippocampus can consolidate every active harness transcript
    Given harness "<harness>" is configured
    When AGM validates Hippocampus transcript parity
    Then harness "<harness>" should have a Hippocampus transcript adapter

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario: Hippocampus LLM consolidation is model-family-neutral
    When AGM validates Hippocampus LLM parity
    Then Hippocampus consolidation should use a model-family-neutral provider
