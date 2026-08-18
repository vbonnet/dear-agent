# SPEC: pkg/benchmarks/SPEC.md
# RELATED-SPEC: pkg/benchmark/SPEC.md
# RELATED-SPEC: pkg/engram/migrate/SPEC.md
# RELATED-SPEC: pkg/hitl/discord/SPEC.md
# RELATED-SPEC: pkg/hitl/github/SPEC.md
Feature: Evaluation and control parity
  Benchmarks, migrations, and human approval transports must preserve shared
  semantics across every harness and model route.

  Scenario Outline: Evaluation and control packages declare SPEC coverage
    Given evaluation control package "<package>" is configured
    When AGM validates evaluation control package coverage
    Then evaluation control package "<package>" should have a co-located SPEC

    Examples:
      | package            |
      | pkg/benchmark      |
      | pkg/benchmarks     |
      | pkg/engram/migrate |
      | pkg/hitl/discord   |
      | pkg/hitl/github    |

  Scenario Outline: Every route preserves benchmark and migration contracts
    Given evaluation harness "<harness>" uses model family "<family>"
    When a shared benchmark task and Engram migration are evaluated
    Then the benchmark should preserve the selected model family "<family>"
    And the migration should remain harness neutral

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
