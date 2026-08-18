# SPEC: pkg/context/SPEC.md
Feature: Context management parity
  Context usage and model-window policy should remain available and explicit
  across every active harness and supported model family.

  Scenario Outline: Every harness and model family has context fallback coverage
    Given context route harness "<harness>" uses model family "<family>"
    When shared context usage is detected without native counters
    Then context usage should preserve the configured model family "<family>"
    And context usage should be marked as estimated
    And context usage should have a positive registered window

    Examples:
      | harness      | family    |
      | claude-code  | anthropic |
      | claude-code  | openai    |
      | claude-code  | gemini    |
      | claude-code  | glm       |
      | claude-code  | deepseek  |
      | claude-code  | nemotron  |
      | claude-code  | qwen      |
      | codex-cli    | anthropic |
      | codex-cli    | openai    |
      | codex-cli    | gemini    |
      | codex-cli    | glm       |
      | codex-cli    | deepseek  |
      | codex-cli    | nemotron  |
      | codex-cli    | qwen      |
      | agy          | anthropic |
      | agy          | openai    |
      | agy          | gemini    |
      | agy          | glm       |
      | agy          | deepseek  |
      | agy          | nemotron  |
      | agy          | qwen      |
      | opencode-cli | anthropic |
      | opencode-cli | openai    |
      | opencode-cli | gemini    |
      | opencode-cli | glm       |
      | opencode-cli | deepseek  |
      | opencode-cli | nemotron  |
      | opencode-cli | qwen      |
      | pi-cli       | anthropic |
      | pi-cli       | openai    |
      | pi-cli       | gemini    |
      | pi-cli       | glm       |
      | pi-cli       | deepseek  |
      | pi-cli       | nemotron  |
      | pi-cli       | qwen      |

  Scenario Outline: Every harness rejects context counters outside the platform integer range
    Given context route harness "<harness>" supplies counters outside the platform integer range
    When shared context usage detection is attempted
    Then context detection should reject the out-of-range counters

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario Outline: Every harness selects competing nested context counters deterministically
    Given context route harness "<harness>" supplies competing nested counters
    When shared context usage detection is attempted
    Then context detection should select the lexically first nested counter set

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |
