# SPEC: agm/internal/mcpparity/SPEC.md
Feature: MCP harness parity
  AGM should expose harness-neutral MCP lifecycle tools. MCP clients should be
  able to create sessions and send messages through the same active harness and
  model-family registry used by the CLI.

  Scenario Outline: Active harnesses support MCP session creation
    Given harness "<harness>" is configured
    When AGM validates MCP session creation parity
    Then harness "<harness>" should have an MCP create-session surface
    And the MCP create-session surface should use shared model validation

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |

  Scenario: Deprecated Gemini keeps MCP compatibility
    Given harness "gemini-cli" is configured
    When AGM validates MCP session creation parity
    Then harness "gemini-cli" should have an MCP create-session surface
    And the MCP create-session surface should be deprecated compatibility

  Scenario Outline: MCP accepts OpenRouter-style model identifiers
    Given harness "opencode-cli" is configured
    When AGM validates MCP model identifier "<model>"
    Then the MCP model identifier should be accepted

    Examples:
      | model                      |
      | z-ai/glm-5.2               |
      | deepseek/deepseek-v4-pro   |
      | nvidia/nemotron-3-ultra    |
      | qwen/qwen3.6-max           |

  Scenario: MCP operation discovery includes lifecycle mutations
    When AGM validates MCP operation discovery parity
    Then the MCP operation registry should expose lifecycle mutations
