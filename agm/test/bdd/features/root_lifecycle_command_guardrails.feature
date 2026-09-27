# SPEC: cmd/mergeloop/SPEC.md
# RELATED-SPEC: internal/mergeloop/SPEC.md
# RELATED-SPEC: cmd/babysit-prs/SPEC.md
# RELATED-SPEC: cmd/bead-close-guard/SPEC.md
# RELATED-SPEC: cmd/bead-pr-guard/SPEC.md
# RELATED-SPEC: cmd/bead-pr-sync/SPEC.md
# RELATED-SPEC: cmd/branch-reaper/SPEC.md
# RELATED-SPEC: cmd/external-pr-reviewer/SPEC.md
# RELATED-SPEC: cmd/merge-audit/SPEC.md
# RELATED-SPEC: cmd/ai-review/SPEC.md
# RELATED-SPEC: internal/prreviewer/SPEC.md
# RELATED-SPEC: cmd/pr-size-audit/SPEC.md
# RELATED-SPEC: internal/prconcern/SPEC.md
# RELATED-SPEC: tools/pr-concern-lint/SPEC.md
# RELATED-SPEC: internal/stackguard/SPEC.md
# RELATED-SPEC: tools/stack-lint/SPEC.md
Feature: Root lifecycle command guardrails
  Repository lifecycle commands should keep executable SPEC traceability, and
  repair-agent routing should remain neutral across active harnesses and model
  families.

  Scenario Outline: Lifecycle command packages declare SPEC coverage
    Given lifecycle command package "<package>" is configured
    When AGM validates lifecycle command package coverage
    Then lifecycle command package "<package>" should have a co-located SPEC

    Examples:
      | package                  |
      | cmd/ai-review            |
      | cmd/babysit-prs          |
      | cmd/bead-close-guard     |
      | cmd/bead-pr-guard        |
      | cmd/bead-pr-sync         |
      | cmd/branch-reaper        |
      | cmd/external-pr-reviewer |
      | cmd/merge-audit          |
      | cmd/mergeloop            |
      | cmd/pr-size-audit        |
      | internal/mergeloop       |
      | internal/prconcern       |
      | internal/prreviewer      |
      | tools/pr-concern-lint    |

  Scenario Outline: Merge repair agents preserve active harness routes
    Given merge repair harness "<harness>" uses model "<model>"
    When AGM builds merge repair session arguments
    Then the merge repair arguments should preserve harness "<harness>" and model "<model>"

    Examples:
      | harness      | model     |
      | claude-code  | sonnet    |
      | codex-cli    | 5.5       |
      | agy          | 3.1-pro-high |
      | opencode-cli | glm-5.2   |
      | pi-cli       | sonnet    |

  Scenario Outline: Merge repair agents preserve model-family routes
    Given merge repair model family "<family>" uses model "<model>"
    When AGM builds merge repair session arguments
    Then the merge repair arguments should preserve model "<model>" for family "<family>"

    Examples:
      | family    | model         |
      | anthropic | opus          |
      | openai    | 5.5           |
      | gemini    | gemini-pro    |
      | glm       | glm-5.2       |
      | deepseek  | deepseek-v4   |
      | nemotron  | nemotron      |
      | qwen      | qwen          |
