# SPEC: pkg/promptcache/SPEC.md
# RELATED-SPEC: pkg/intent/SPEC.md
# RELATED-SPEC: pkg/notify/SPEC.md
# RELATED-SPEC: pkg/output-formatter/SPEC.md
# RELATED-SPEC: pkg/phaseengram/SPEC.md
# RELATED-SPEC: pkg/selfimprove/SPEC.md
# RELATED-SPEC: pkg/signals/SPEC.md
# RELATED-SPEC: pkg/stats/SPEC.md
# RELATED-SPEC: pkg/stophook/SPEC.md
# RELATED-SPEC: pkg/synchub/SPEC.md
# RELATED-SPEC: pkg/trigger/SPEC.md
Feature: Agent utility parity
  Shared agent utilities must retain their own behavioral contracts and route
  through neutral hook, cache, and synchronization boundaries.

  Scenario Outline: Agent utility packages declare SPEC coverage
    Given agent utility package "<package>" is configured
    When AGM validates agent utility package coverage
    Then agent utility package "<package>" should have a co-located SPEC

    Examples:
      | package              |
      | pkg/intent           |
      | pkg/notify           |
      | pkg/output-formatter |
      | pkg/phaseengram      |
      | pkg/promptcache      |
      | pkg/selfimprove      |
      | pkg/signals          |
      | pkg/stats            |
      | pkg/stophook         |
      | pkg/synchub          |
      | pkg/trigger          |

  Scenario Outline: Every harness and model family uses neutral utility contracts
    Given agent utility harness "<harness>" uses model family "<family>"
    When shared agent utility contracts are resolved
    Then the cache policy should preserve model family "<family>"
    And the hook input should preserve harness "<harness>"
    And the synchronization session should remain route neutral

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
