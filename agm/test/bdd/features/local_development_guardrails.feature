# SPEC: internal/safegit/SPEC.md
# RELATED-SPEC: internal/safepr/SPEC.md
# RELATED-SPEC: internal/safesrc/SPEC.md
# RELATED-SPEC: internal/safeunlock/SPEC.md
# RELATED-SPEC: cmd/safe-push/SPEC.md
# RELATED-SPEC: cmd/safe-pr/SPEC.md
# RELATED-SPEC: cmd/safe-merge/SPEC.md
# RELATED-SPEC: cmd/safe-rebase/SPEC.md
# RELATED-SPEC: cmd/safe-unlock/SPEC.md
# RELATED-SPEC: cmd/test-affected/SPEC.md
# RELATED-SPEC: scripts/SPEC.md
# RELATED-SPEC: wayfinder/pkg/sandbox/SPEC.md
# RELATED-SPEC: agm/cmd/agm/SPEC.md
Feature: Local development guardrails
  Agent development should use audited local wrappers for push, PR, merge,
  rebase, and stale-lock cleanup instead of raw git or GitHub mutation commands.

  Scenario Outline: Safe local development commands declare SPEC coverage
    Given safe local development command "<command>" is configured
    When AGM validates safe local development command coverage
    Then safe local development command "<command>" should have a co-located SPEC

    Examples:
      | command     |
      | safe-push   |
      | safe-pr     |
      | safe-merge  |
      | safe-rebase |
      | safe-unlock |

  Scenario Outline: Safe local development libraries declare SPEC coverage
    Given safe local development library "<package>" is configured
    When AGM validates safe local development library coverage
    Then safe local development library "<package>" should have a co-located SPEC

    Examples:
      | package    |
      | safegit    |
      | safepr     |
      | safesrc    |
      | safeunlock |

  Scenario Outline: Canonical Wayfinder traces are accepted across parity routes
    Given canonical Wayfinder V2 status for harness "<harness>" and model family "<family>"
    When safe-pr loads the canonical planning trace
    Then safe-pr should attribute the trace to project "parity-audit"

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

  Scenario: Safe PR creation allows the repository full gate to finish
    Given the safe-pr full preflight timeout is configured
    When AGM validates the safe-pr preflight budget
    Then safe-pr should allow at least 60 minutes for preflight-full

  Scenario Outline: Safe PR creation preserves worktree lock ownership
    Given a safe-pr linked worktree with "<initial_lock>" lock ownership
    When safe-pr protects a "<outcome>" preflight and PR creation transaction
    Then the worktree should be protected during preflight and PR creation
    And the "<final_lock>" lock ownership should remain after the transaction

    Examples:
      | initial_lock  | outcome | final_lock   |
      | absent        | success | absent       |
      | absent        | failure | absent       |
      | pre-existing  | success | pre-existing |
      | pre-existing  | failure | pre-existing |
      | stale-safe-pr | success | absent       |

  Scenario: Wayfinder cleanup preserves a safe-pr-protected worktree
    Given a safe-pr linked worktree with "absent" lock ownership
    When Wayfinder cleanup overlaps a protected safe-pr transaction
    Then Wayfinder should preserve the protected worktree and reject cleanup
    And Wayfinder should remove the worktree after the safe-pr transaction

  Scenario: Automated cleanup preserves active Git worktree locks
    When AGM runs the protected cleanup regressions
    Then Wayfinder and AGM cleanup should preserve Git-locked checkouts

  Scenario: Repository cleanup preserves branches for protected worktrees
    When AGM runs the protected repository cleanup regression
    Then repository cleanup should preserve the worktree and its branches

  Scenario: Safe PR child lifetime survives abrupt parent termination
    When AGM runs the safe-pr abrupt-parent regression
    Then the child should retain transaction ownership until it exits

  Scenario: Safe PR audit records the final transaction outcome
    When AGM runs the safe-pr final transaction audit regression
    Then each safe-pr transaction should have one accurate audit record

  Scenario: Safe merge distinguishes required checks from advisory history
    When AGM runs the effective required-check regressions
    Then safe-merge should enforce complete provider-required CI without advisory drift

  Scenario: All repository test runners use the required CI timeout
    Given local, affected integration, and required CI Go test timeouts are configured
    When AGM validates Go test timeout parity
    Then all repository Go test timeouts should match
    And affected integration deadline layers should preserve their nested budgets

  Scenario: Local and required CI vulnerability policy stay aligned
    Given local and required CI govulncheck allowlists are configured
    When AGM validates govulncheck policy parity
    Then the local and required CI govulncheck allowlists should match
