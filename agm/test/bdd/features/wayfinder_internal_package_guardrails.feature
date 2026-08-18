# SPEC: wayfinder/cmd/wayfinder/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/beads/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/config/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/git/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/history/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/lintcontext/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/telemetry/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/tracker/SPEC.md
# RELATED-SPEC: wayfinder/internal/project/SPEC.md
# RELATED-SPEC: wayfinder/internal/tracker/SPEC.md
# RELATED-SPEC: wayfinder/lib/presets/SPEC.md
Feature: Wayfinder internal package guardrails
  The canonical Wayfinder V2 implementation must retain executable contracts
  across every supported harness and model family.

  Scenario Outline: Wayfinder internal packages declare SPEC coverage
    Given Wayfinder internal package "<package>" is configured
    When AGM validates Wayfinder internal package coverage
    Then Wayfinder internal package "<package>" should have a co-located SPEC

    Examples:
      | package                                                  |
      | wayfinder/cmd/wayfinder                                  |
      | wayfinder/cmd/wayfinder-session/internal/beads           |
      | wayfinder/cmd/wayfinder-session/internal/config          |
      | wayfinder/cmd/wayfinder-session/internal/git             |
      | wayfinder/cmd/wayfinder-session/internal/history         |
      | wayfinder/cmd/wayfinder-session/internal/lintcontext     |
      | wayfinder/cmd/wayfinder-session/internal/telemetry       |
      | wayfinder/cmd/wayfinder-session/internal/tracker         |
      | wayfinder/internal/project                               |
      | wayfinder/internal/tracker                               |
      | wayfinder/lib/presets                                    |

  Scenario Outline: Canonical Wayfinder contracts are provider neutral
    Given canonical Wayfinder is driven by harness "<harness>" and model family "<family>"
    When AGM validates the Wayfinder parity route
    Then Wayfinder should preserve the same nine-phase contract

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
