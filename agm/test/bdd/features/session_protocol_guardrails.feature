# SPEC: pkg/a2a/SPEC.md
# RELATED-SPEC: pkg/a2a/client/SPEC.md
# RELATED-SPEC: pkg/acceptance/SPEC.md
# RELATED-SPEC: pkg/agenttrace/SPEC.md
# RELATED-SPEC: pkg/aggregator/SPEC.md
# RELATED-SPEC: pkg/aggregator/collectors/SPEC.md
# RELATED-SPEC: pkg/aggregator/salience/SPEC.md
Feature: Session protocol and signal guardrails
  Shared A2A transport, acceptance, tracing, and project-signal packages should
  keep executable contracts across every supported harness and model family.

  Scenario Outline: Session protocol packages declare SPEC coverage
    Given session protocol package "<package>" is configured
    When AGM validates session protocol package coverage
    Then session protocol package "<package>" should have a co-located SPEC

    Examples:
      | package                   |
      | pkg/a2a                   |
      | pkg/a2a/client            |
      | pkg/acceptance            |
      | pkg/agenttrace            |
      | pkg/aggregator            |
      | pkg/aggregator/collectors |
      | pkg/aggregator/salience   |

  Scenario Outline: A2A cards preserve harness identity without model coupling
    Given harness "<harness>" and model family "<family>" expose an A2A session
    When the shared A2A session card is built
    Then the A2A card should advertise only harness "<harness>"
    And the A2A card presentation should not encode model family "<family>"

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

  Scenario: Trace redaction traverses all nested values with stable normalization
    Given agent trace redaction policy is configured
    When AGM validates nested trace redaction traversal
    Then every nested trace value should be inspected without per-key normalizer allocation
