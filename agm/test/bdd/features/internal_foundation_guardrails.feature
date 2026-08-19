# SPEC: internal/override/SPEC.md
# RELATED-SPEC: internal/baseline/SPEC.md
# RELATED-SPEC: internal/benchmark/SPEC.md
# RELATED-SPEC: internal/buildstamp/SPEC.md
# RELATED-SPEC: internal/ci/act/SPEC.md
# RELATED-SPEC: internal/common/SPEC.md
# RELATED-SPEC: internal/drift/SPEC.md
# RELATED-SPEC: internal/earslint/SPEC.md
# RELATED-SPEC: internal/fileutil/SPEC.md
# RELATED-SPEC: internal/gittest/SPEC.md
# RELATED-SPEC: internal/logrotate/SPEC.md
# RELATED-SPEC: internal/sqlite/SPEC.md
# RELATED-SPEC: internal/tracking/SPEC.md
# RELATED-SPEC: internal/vroomgate/SPEC.md
Feature: Internal foundation guardrails
  Shared benchmark, persistence, safety, and host-operation packages should
  carry executable specifications. Their policy contracts must remain stable
  across every supported harness and model family.

  Scenario Outline: Internal foundation packages declare SPEC coverage
    Given internal foundation package "<package>" is configured
    When AGM validates internal foundation package coverage
    Then internal foundation package "<package>" should have a co-located SPEC

    Examples:
      | package            |
      | internal/baseline  |
      | internal/benchmark |
      | internal/buildstamp |
      | internal/ci/act    |
      | internal/common    |
      | internal/drift     |
      | internal/earslint  |
      | internal/fileutil  |
      | internal/gittest   |
      | internal/logrotate |
      | internal/override  |
      | internal/sqlite    |
      | internal/tracking  |

  Scenario Outline: High-risk override policy is harness and model neutral
    Given harness "<harness>" and model family "<family>" use the shared override policy
    When the high-risk override policy evaluates concrete and filler reasons
    Then the override judge identity should be provider neutral
    And the deterministic override floor should remain mandatory

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

  Scenario: Test-created Git repositories cannot execute host hooks
    Given the host global Git configuration installs a canary hook
    When AGM runs the hermetic Git sandbox regressions
    Then the unisolated control should prove the canary hook fires
    And no sandboxed repository should execute a host hook
