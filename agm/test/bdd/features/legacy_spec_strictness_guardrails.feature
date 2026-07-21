# SPEC: agm/SPEC.md
# RELATED-SPEC: agm/cmd/agm/workspace/SPEC.md
# RELATED-SPEC: agm/cmd/agm-daemon/SPEC.md
# RELATED-SPEC: agm/internal/agent/gemini/SPEC.md
# RELATED-SPEC: agm/internal/dolt/SPEC.md
# RELATED-SPEC: agm/internal/evaluation/SPEC.md
# RELATED-SPEC: cmd/vroom-governor/SPEC.md
# RELATED-SPEC: engram/SPEC.md
# RELATED-SPEC: engram/cmd/engram/SPEC.md
# RELATED-SPEC: engram/cmd/engram/cmd/SPEC.md
# RELATED-SPEC: engram/errormemory/SPEC.md
# RELATED-SPEC: engram/hooks-bin/cmd/generate-patterns/SPEC.md
# RELATED-SPEC: engram/internal/health/SPEC.md
# RELATED-SPEC: engram/mcp/SPEC.md
# RELATED-SPEC: engram/retrieval/SPEC.md
# RELATED-SPEC: internal/ci/SPEC.md
# RELATED-SPEC: internal/sandbox/SPEC.md
# RELATED-SPEC: pkg/cliframe/SPEC.md
# RELATED-SPEC: pkg/engram/SPEC.md
# RELATED-SPEC: pkg/hash/SPEC.md
# RELATED-SPEC: pkg/llm/SPEC.md
# RELATED-SPEC: pkg/progress/SPEC.md
# RELATED-SPEC: pkg/table/SPEC.md
# RELATED-SPEC: tools/benchmark-query/SPEC.md
# RELATED-SPEC: tools/devlog/SPEC.md
# RELATED-SPEC: tools/schema-registry/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/SPEC.md
Feature: Legacy specification strictness guardrails
  Maintained legacy specifications must converge on the same executable EARS
  and cross-provider traceability contract as newly governed packages.

  Scenario Outline: Legacy specifications expose strict executable requirements
    Given legacy specification "<spec>" is selected
    When AGM validates the selected legacy specification
    Then the legacy specification should pass strict EARS lint
    And the legacy specification should reference its executable guardrail

    Examples:
      | spec                                                   |
      | agm/SPEC.md                                            |
      | agm/cmd/agm/workspace/SPEC.md                          |
      | agm/cmd/agm-daemon/SPEC.md                             |
      | agm/internal/agent/gemini/SPEC.md                      |
      | agm/internal/dolt/SPEC.md                              |
      | agm/internal/evaluation/SPEC.md                        |
      | cmd/vroom-governor/SPEC.md                             |
      | engram/SPEC.md                                         |
      | engram/cmd/engram/SPEC.md                              |
      | engram/cmd/engram/cmd/SPEC.md                          |
      | engram/errormemory/SPEC.md                             |
      | engram/hooks-bin/cmd/generate-patterns/SPEC.md         |
      | engram/internal/health/SPEC.md                         |
      | engram/mcp/SPEC.md                                     |
      | engram/retrieval/SPEC.md                               |
      | internal/ci/SPEC.md                                    |
      | internal/sandbox/SPEC.md                               |
      | pkg/cliframe/SPEC.md                                   |
      | pkg/engram/SPEC.md                                     |
      | pkg/hash/SPEC.md                                       |
      | pkg/llm/SPEC.md                                        |
      | pkg/progress/SPEC.md                                   |
      | pkg/table/SPEC.md                                      |
      | tools/benchmark-query/SPEC.md                          |
      | tools/devlog/SPEC.md                                   |
      | tools/schema-registry/SPEC.md                          |
      | wayfinder/cmd/wayfinder-session/SPEC.md                |

  Scenario Outline: Legacy specification enforcement is provider-neutral
    Given legacy specification coverage runs through "<harness>" with "<family>"
    When AGM validates legacy specification route parity
    Then every selected legacy specification should retain strict executable coverage

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
