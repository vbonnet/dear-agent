# SPEC: pkg/vroom/escalation/SPEC.md
# RELATED-SPEC: pkg/vroom/admission/SPEC.md
# RELATED-SPEC: pkg/vroom/decisiontrail/SPEC.md
# RELATED-SPEC: pkg/vroom/goplswatch/SPEC.md
# RELATED-SPEC: pkg/vroom/supervisor/SPEC.md
# RELATED-SPEC: pkg/vroom/vroom/SPEC.md
Feature: VROOM runtime guardrails
  VROOM coordination must keep durable specifications and use the same
  provider-neutral contracts across every harness and model family.

  Scenario Outline: VROOM runtime packages declare SPEC coverage
    Given VROOM runtime package "<package>" is configured
    When AGM validates VROOM runtime package coverage
    Then VROOM runtime package "<package>" should have a co-located SPEC

    Examples:
      | package                     |
      | pkg/vroom/admission         |
      | pkg/vroom/decisiontrail     |
      | pkg/vroom/escalation        |
      | pkg/vroom/goplswatch        |
      | pkg/vroom/supervisor        |
      | pkg/vroom/vroom             |

  Scenario Outline: Every harness preserves every model family route
    Given VROOM harness "<harness>" uses model family "<family>" route "<model>"
    When VROOM builds the shared adjudication and worker dispatch contracts
    Then VROOM adjudication should be attributed to model family "<family>"
    And VROOM worker dispatch should preserve model route "<model>"
    And VROOM contracts should remain independent of harness "<harness>"

    Examples:
      | harness      | family    | model               |
      | claude-code  | anthropic | claude-sonnet-4-5   |
      | claude-code  | openai    | gpt-5.2-codex       |
      | claude-code  | gemini    | gemini-3-pro        |
      | claude-code  | glm       | glm-5.2             |
      | claude-code  | deepseek  | deepseek-v4         |
      | claude-code  | nemotron  | nemotron-4          |
      | claude-code  | qwen      | qwen3-coder         |
      | codex-cli    | anthropic | claude-sonnet-4-5   |
      | codex-cli    | openai    | gpt-5.2-codex       |
      | codex-cli    | gemini    | gemini-3-pro        |
      | codex-cli    | glm       | glm-5.2             |
      | codex-cli    | deepseek  | deepseek-v4         |
      | codex-cli    | nemotron  | nemotron-4          |
      | codex-cli    | qwen      | qwen3-coder         |
      | agy          | anthropic | claude-sonnet-4-5   |
      | agy          | openai    | gpt-5.2-codex       |
      | agy          | gemini    | gemini-3-pro        |
      | agy          | glm       | glm-5.2             |
      | agy          | deepseek  | deepseek-v4         |
      | agy          | nemotron  | nemotron-4          |
      | agy          | qwen      | qwen3-coder         |
      | opencode-cli | anthropic | claude-sonnet-4-5   |
      | opencode-cli | openai    | gpt-5.2-codex       |
      | opencode-cli | gemini    | gemini-3-pro        |
      | opencode-cli | glm       | glm-5.2             |
      | opencode-cli | deepseek  | deepseek-v4         |
      | opencode-cli | nemotron  | nemotron-4          |
      | opencode-cli | qwen      | qwen3-coder         |
      | pi-cli       | anthropic | claude-sonnet-4-6   |
      | pi-cli       | openai    | gpt-5.6-terra       |
      | pi-cli       | gemini    | gemini-3.5-flash    |
      | pi-cli       | glm       | glm-5.2             |
      | pi-cli       | deepseek  | deepseek-v4         |
      | pi-cli       | nemotron  | nemotron            |
      | pi-cli       | qwen      | qwen                |

  Scenario Outline: Harnesses delegate unspecified model selection
    Given VROOM harness "<harness>" has no explicit model route
    When VROOM builds default worker dispatch arguments
    Then VROOM worker dispatch should omit a fixed model route

    Examples:
      | harness     |
      | claude-code |
      | codex-cli   |
      | agy         |
      | opencode-cli |
      | pi-cli       |

  Scenario: Removed queue tasks release retained storage
    When AGM validates VROOM queue storage hygiene
    Then the VROOM supervisor specification should require cleared backing storage
