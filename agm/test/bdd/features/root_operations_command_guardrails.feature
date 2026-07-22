# SPEC: cmd/vroom-prompt-gen/SPEC.md
# RELATED-SPEC: internal/vroomprompt/SPEC.md
# RELATED-SPEC: cmd/agm-webhook-receiver/SPEC.md
# RELATED-SPEC: cmd/benchmark-baseline/SPEC.md
# RELATED-SPEC: cmd/dear-agent-bumblebee/SPEC.md
# RELATED-SPEC: cmd/flywheel-drift/SPEC.md
# RELATED-SPEC: cmd/retro-audit/SPEC.md
Feature: Root operations command guardrails
  Host operations commands should keep executable SPEC traceability, and VROOM
  prompts should preserve active harness and model-family routes.

  Scenario Outline: Operations command packages declare SPEC coverage
    Given operations command package "<package>" is configured
    When AGM validates operations command package coverage
    Then operations command package "<package>" should have a co-located SPEC

    Examples:
      | package                  |
      | cmd/agm-webhook-receiver |
      | cmd/benchmark-baseline   |
      | cmd/dear-agent-bumblebee |
      | cmd/flywheel-drift       |
      | cmd/retro-audit          |
      | cmd/vroom-prompt-gen     |
      | internal/vroomprompt     |

  Scenario Outline: VROOM prompts preserve active harness routes
    Given VROOM worker harness "<harness>" uses model "<model>"
    When AGM renders the VROOM worker route
    Then the VROOM worker rule should preserve harness "<harness>" and model "<model>"

    Examples:
      | harness      | model   |
      | claude-code  | sonnet  |
      | codex-cli    | 5.5     |
      | agy          | 3.1-pro-high |
      | opencode-cli | glm-5.2 |
      | pi-cli       | sonnet  |

  Scenario Outline: VROOM prompts preserve supported model families
    Given VROOM worker model family "<family>" uses model "<model>"
    When AGM renders the VROOM worker route
    Then the VROOM worker rule should preserve model "<model>" for family "<family>"

    Examples:
      | family    | model       |
      | anthropic | opus        |
      | openai    | 5.5         |
      | gemini    | gemini-pro  |
      | glm       | glm-5.2     |
      | deepseek  | deepseek-v4 |
      | nemotron  | nemotron    |
      | qwen      | qwen        |
