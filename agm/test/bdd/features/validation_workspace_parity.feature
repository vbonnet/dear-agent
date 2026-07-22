# SPEC: pkg/security/SPEC.md
# RELATED-SPEC: pkg/validation/SPEC.md
# RELATED-SPEC: pkg/validation/engram/SPEC.md
# RELATED-SPEC: pkg/validation/scope/SPEC.md
# RELATED-SPEC: pkg/validator/SPEC.md
# RELATED-SPEC: pkg/vcs/SPEC.md
# RELATED-SPEC: pkg/version/SPEC.md
# RELATED-SPEC: pkg/workspace/SPEC.md
# RELATED-SPEC: pkg/workspace/dolt/SPEC.md
Feature: Validation and workspace parity
  Validation, repository, workspace, and filesystem safety packages are shared
  below harness adapters and must not acquire route-specific defaults.

  Scenario Outline: Validation and workspace packages declare SPEC coverage
    Given validation workspace package "<package>" is configured
    When AGM validates validation workspace package coverage
    Then validation workspace package "<package>" should have a co-located SPEC

    Examples:
      | package               |
      | pkg/security          |
      | pkg/validation        |
      | pkg/validation/engram |
      | pkg/validation/scope  |
      | pkg/validator         |
      | pkg/vcs               |
      | pkg/version           |
      | pkg/workspace         |
      | pkg/workspace/dolt    |

  Scenario Outline: Validation and workspace behavior stays neutral on every route
    Given validation workspace harness "<harness>" uses model family "<family>"
    When AGM scans validation workspace packages for route defaults
    Then validation workspace behavior should remain route neutral

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
