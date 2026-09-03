# SPEC: internal/agenticreview/SPEC.md
# RELATED-SPEC: cmd/agentic-review-gate/SPEC.md
# RELATED-SPEC: .github/actions/agentic-review-label/SPEC.md
# RELATED-SPEC: .github/actions/agentic-review-verdict/SPEC.md
Feature: Agentic review gate guardrails
  A merge must wait for every reviewer family's own verdict, degrade around a
  family that genuinely cannot report, and never treat one family's approval as
  another's.

  Scenario: Every reviewer family approves
    Given the reviewer families claude, codex and gemini
    When claude approves, codex approves and gemini approves
    Then the agentic review gate should permit the merge

  Scenario: One family requesting changes is not masked by the others
    Given the reviewer families claude, codex and gemini
    When claude approves, codex requests changes and gemini approves
    Then the agentic review gate should refuse the merge
    And the refusal should name codex

  Scenario: A quorum carries the merge past one down family
    Given the reviewer families claude, codex and gemini
    When claude approves, codex reports an error and gemini approves
    Then the agentic review gate should permit the merge

  Scenario: A ready pull request with no review started cannot merge
    Given the reviewer families claude, codex and gemini
    When no family has started reviewing
    Then the agentic review gate should refuse the merge

  Scenario: A reviewer that started but has not reported holds the merge
    Given the reviewer families claude, codex and gemini
    When claude approves, gemini approves and codex has only started
    Then the agentic review gate should refuse the merge

  Scenario: Requested changes outrank a satisfied quorum
    Given the reviewer families claude, codex and gemini
    When claude approves, codex requests changes and gemini reports an error
    Then the agentic review gate should refuse the merge
