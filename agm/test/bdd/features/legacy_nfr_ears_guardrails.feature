# SPEC: engram/ecphory/SPEC.md
# RELATED-SPEC: engram/ecphory/ranking/SPEC.md
# RELATED-SPEC: pkg/monitoring/SPEC.md
# RELATED-SPEC: pkg/telemetry/SPEC.md
# RELATED-SPEC: tools/dod-enforcer/SPEC.md
Feature: Legacy NFR EARS guardrails
  Historical functional and non-functional requirements must retain their
  meaning and identifiers while conforming to strict executable EARS syntax.

  Scenario Outline: Converted legacy requirements pass strict EARS lint
    Given converted legacy specification "<spec>" is selected
    When AGM validates converted legacy requirements
    Then the converted legacy specification should pass strict EARS lint
    And the converted legacy specification should reference its executable guardrail

    Examples:
      | spec                             |
      | engram/ecphory/SPEC.md           |
      | engram/ecphory/ranking/SPEC.md   |
      | pkg/monitoring/SPEC.md            |
      | pkg/telemetry/SPEC.md             |
      | tools/dod-enforcer/SPEC.md        |

  Scenario Outline: Converted requirements remain provider-neutral
    Given converted legacy coverage runs through "<harness>" with "<family>"
    When AGM validates converted legacy route parity
    Then every converted legacy specification should retain strict executable coverage

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
