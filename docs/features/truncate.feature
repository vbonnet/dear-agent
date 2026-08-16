# SPEC: wayfinder/cmd/wayfinder-session/internal/truncate/SPEC.md
Feature: Truncation
  The system should provide deterministic output compression for LLM contexts.

  Scenario: Truncate output
    Given an output string that is too long
    When it is truncated
    Then it should align to line breaks
    And it should insert an omission marker
