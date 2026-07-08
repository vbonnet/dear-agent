# SPEC: internal/safegit/SPEC.md
# RELATED-SPEC: internal/safepr/SPEC.md
# RELATED-SPEC: internal/safesrc/SPEC.md
# RELATED-SPEC: internal/safeunlock/SPEC.md
# RELATED-SPEC: cmd/safe-push/SPEC.md
# RELATED-SPEC: cmd/safe-pr/SPEC.md
# RELATED-SPEC: cmd/safe-merge/SPEC.md
# RELATED-SPEC: cmd/safe-rebase/SPEC.md
# RELATED-SPEC: cmd/safe-unlock/SPEC.md
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
