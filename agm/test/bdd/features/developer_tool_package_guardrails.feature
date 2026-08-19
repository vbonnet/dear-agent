# SPEC: cmd/session-skill-extractor/SPEC.md
# RELATED-SPEC: pkg/headerlint/SPEC.md
# RELATED-SPEC: pkg/instructionlint/SPEC.md
# RELATED-SPEC: tests/githooks/SPEC.md
# RELATED-SPEC: tools/ci-drift-guard/SPEC.md
# RELATED-SPEC: tools/dead-links/SPEC.md
# RELATED-SPEC: tools/instruction-lint/SPEC.md
# RELATED-SPEC: tools/language-policy/SPEC.md
# RELATED-SPEC: tools/header-lint/SPEC.md
# RELATED-SPEC: tools/devlog/cmd/devlog/SPEC.md
# RELATED-SPEC: tools/devlog/internal/config/SPEC.md
# RELATED-SPEC: tools/devlog/internal/errors/SPEC.md
# RELATED-SPEC: tools/devlog/internal/git/SPEC.md
# RELATED-SPEC: tools/devlog/internal/output/SPEC.md
# RELATED-SPEC: tools/devlog/internal/workspace/SPEC.md
# RELATED-SPEC: tools/dod-enforcer/hooks/cmd/stop-quality-guard/SPEC.md
# RELATED-SPEC: tools/schema-registry/internal/mcp/SPEC.md
# RELATED-SPEC: tools/schema-registry/internal/query/SPEC.md
# RELATED-SPEC: tools/schema-registry/internal/registry/SPEC.md
# RELATED-SPEC: tools/schema-registry/internal/schema/SPEC.md
Feature: Developer tool package guardrails
  Repository support tools must keep executable specifications, and their
  shared contracts must remain available to every supported harness and model
  family without requiring a provider-specific control path.

  Scenario Outline: Developer tool packages declare SPEC coverage
    Given developer tool package "<package>" is configured
    When AGM validates developer tool package coverage
    Then developer tool package "<package>" should have a co-located SPEC

    Examples:
      | package                                            |
      | cmd/session-skill-extractor                        |
      | pkg/headerlint                                     |
      | pkg/instructionlint                                |
      | tests/githooks                                     |
      | tools/ci-drift-guard                               |
      | tools/dead-links                                   |
      | tools/instruction-lint                             |
      | tools/header-lint                                  |
      | tools/devlog/cmd/devlog                            |
      | tools/devlog/internal/config                       |
      | tools/devlog/internal/errors                       |
      | tools/devlog/internal/git                          |
      | tools/devlog/internal/output                       |
      | tools/devlog/internal/workspace                    |
      | tools/dod-enforcer/hooks/cmd/stop-quality-guard    |
      | tools/schema-registry/internal/mcp                 |
      | tools/schema-registry/internal/query               |
      | tools/schema-registry/internal/registry            |
      | tools/schema-registry/internal/schema              |

  Scenario Outline: Shared developer tooling is provider neutral
    Given developer tooling is invoked by harness "<harness>" with model family "<family>"
    When AGM validates the developer tooling parity route
    Then the developer tooling contract should remain provider neutral

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
