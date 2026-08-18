# SPEC: pkg/autoconfig/SPEC.md
# RELATED-SPEC: pkg/backlog/SPEC.md
# RELATED-SPEC: pkg/codegen/SPEC.md
# RELATED-SPEC: pkg/codeintel/SPEC.md
# RELATED-SPEC: pkg/config-loader/SPEC.md
# RELATED-SPEC: pkg/enforcement/SPEC.md
# RELATED-SPEC: pkg/evalcase/SPEC.md
# RELATED-SPEC: pkg/eventbus/SPEC.md
# RELATED-SPEC: pkg/frontmatter/SPEC.md
# RELATED-SPEC: pkg/gracefulexit/SPEC.md
# RELATED-SPEC: pkg/health-checker/SPEC.md
Feature: Shared runtime policy package guardrails
  Shared parsing, policy, analysis, generation, and health packages must keep
  executable specifications and must not silently introduce harness-specific
  or model-family-specific behavior.

  Scenario Outline: Shared runtime policy packages declare route-neutral coverage
    Given shared runtime policy package "<package>" is configured
    When AGM validates shared runtime policy package coverage
    Then shared runtime policy package "<package>" should have a co-located SPEC
    And shared runtime policy package "<package>" should cover every supported harness and model family
    And shared runtime policy package "<package>" should not embed a harness or model-family route

    Examples:
      | package            |
      | pkg/autoconfig     |
      | pkg/backlog        |
      | pkg/codegen        |
      | pkg/codeintel      |
      | pkg/config-loader  |
      | pkg/enforcement    |
      | pkg/evalcase       |
      | pkg/eventbus       |
      | pkg/frontmatter    |
      | pkg/gracefulexit   |
      | pkg/health-checker |

  Scenario Outline: Shared runtime policy remains neutral on every route
    Given shared runtime route harness "<harness>" uses model family "<family>"
    When AGM validates every shared runtime policy package for that route
    Then no shared runtime policy package should embed route-specific behavior

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
