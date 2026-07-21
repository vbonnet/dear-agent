# SPEC: agm/internal/quotaparity/SPEC.md
# RELATED-SPEC: agm/internal/manifest/SPEC.md
# RELATED-SPEC: agm/internal/budget/SPEC.md
# RELATED-SPEC: agm/internal/usage/SPEC.md
# RELATED-SPEC: internal/pricing/SPEC.md
# RELATED-SPEC: internal/tokens/SPEC.md
# RELATED-SPEC: internal/tokens/tokenizers/SPEC.md
Feature: Quota monitoring parity
  AGM should expose truthful context, cost, and quota monitoring behavior for
  every active harness and every supported model family. Missing native data
  should be explicit rather than replaced with Claude-specific defaults.

  Scenario Outline: Active harnesses publish quota monitoring surfaces
    Given harness "<harness>" is configured
    When AGM validates quota monitoring parity
    Then harness "<harness>" should have a context quota source
    And harness "<harness>" should have a cost quota source
    And harness "<harness>" should have a rate limit quota policy

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario Outline: Model families have quota pricing policy
    Given model family "<family>" is configured
    When AGM validates quota model family coverage
    Then model family "<family>" should have a quota pricing policy
    And model family "<family>" should have a default quota model route

    Examples:
      | family    |
      | anthropic |
      | openai    |
      | gemini    |
      | glm       |
      | deepseek  |
      | nemotron  |
      | qwen      |

  Scenario Outline: Priority model families have sourced shared pricing
    Given model family "<family>" is configured
    When AGM validates quota model family coverage
    Then model family "<family>" should have sourced shared pricing

    Examples:
      | family    |
      | glm       |
      | deepseek  |
      | nemotron  |
      | qwen      |
