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

  Scenario: A native provider model outside AGM's static window table honors its override
    Given a managed Pi transcript uses provider "openai" model "gpt-4.1"
    And the Pi model catalog overrides "openai/gpt-4.1" to 4096 tokens
    When AGM detects the managed Pi context
    Then the Pi context should report 3562 of 4096 tokens used
    And the Pi context model should be "openai/gpt-4.1"

  Scenario: A nested OpenRouter vendor route retains its native context window
    Given a managed Pi transcript uses provider "openrouter" model "openai/gpt-5.4"
    When AGM detects the managed Pi context
    Then the Pi context should report 3562 of 1050000 tokens used
    And the Pi context model should be "openrouter/openai/gpt-5.4"

  Scenario: A custom model ID that begins with its provider remains opaque
    Given a managed Pi transcript uses provider "acme" model "acme/foo"
    And the Pi custom model catalog for provider "acme" declares model "acme/foo" with an 8192 token window
    When AGM detects the managed Pi context
    Then the Pi context should report 3562 of 8192 tokens used
    And the Pi context model should be "acme/acme/foo"
