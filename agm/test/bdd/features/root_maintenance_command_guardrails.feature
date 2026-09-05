# SPEC: cmd/burndown-maint/SPEC.md
# RELATED-SPEC: internal/burndownmaint/SPEC.md
# RELATED-SPEC: cmd/pr-linkify/SPEC.md
# RELATED-SPEC: internal/prlinkify/SPEC.md
# RELATED-SPEC: cmd/src-recovery/SPEC.md
# RELATED-SPEC: cmd/install-post-merge-hook/SPEC.md
# RELATED-SPEC: cmd/chezmoi-deploy/SPEC.md
Feature: Root maintenance command guardrails
  Host maintenance commands should keep executable SPEC traceability and route
  AGM workers without harness-specific credential or model assumptions.

  Scenario Outline: Maintenance command packages declare SPEC coverage
    Given maintenance command package "<package>" is configured
    When AGM validates maintenance command package coverage
    Then maintenance command package "<package>" should have a co-located SPEC

    Examples:
      | package                     |
      | cmd/burndown-maint          |
      | internal/burndownmaint      |
      | cmd/pr-linkify              |
      | internal/prlinkify          |
      | cmd/src-recovery            |
      | cmd/install-post-merge-hook |
      | cmd/chezmoi-deploy          |

  Scenario Outline: Burndown workers preserve active harness routes
    Given burndown worker harness "<harness>" uses model "<model>"
    When AGM builds burndown worker session arguments
    Then the burndown arguments should preserve harness "<harness>" and model "<model>"

    Examples:
      | harness      | model   |
      | claude-code  | sonnet  |
      | codex-cli    | 5.5     |
      | agy          | 3.1-pro-high |
      | opencode-cli | glm-5.2 |
      | pi-cli       | sonnet  |

  Scenario Outline: Burndown workers preserve supported model families
    Given burndown worker model family "<family>" uses model "<model>"
    When AGM builds burndown worker session arguments
    Then the burndown arguments should preserve model "<model>" for family "<family>"

    Examples:
      | family    | model       |
      | anthropic | opus        |
      | openai    | 5.5         |
      | gemini    | gemini-pro  |
      | glm       | glm-5.2     |
      | deepseek  | deepseek-v4 |
      | nemotron  | nemotron    |
      | qwen      | qwen        |

  Scenario: Burndown AGM subprocesses compose signal cancellation and timeouts
    Given burndown maintenance subprocess policy is configured
    When AGM validates burndown subprocess cancellation
    Then session listing and worker spawning should use timeout-bounded signal context
