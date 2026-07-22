# SPEC: agm/internal/session/SPEC.md
Feature: Pi custom model context
  AGM should report Pi's native context against the same custom model window
  that Pi uses without executing unrelated catalog configuration.

  Scenario: A provider-qualified custom model uses its configured context window
    Given a managed Pi transcript uses provider "ollama" model "qwen2.5-coder:7b"
    And the Pi custom model catalog declares an 8192 token window with an inert credential command
    When AGM detects the managed Pi context
    Then the Pi context should report 3562 of 8192 tokens used
    And the Pi context model should be "ollama/qwen2.5-coder:7b"
    And the Pi catalog credential command should remain inert
