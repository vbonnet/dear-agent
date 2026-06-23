Feature: Harness parity
  AGM should use one harness-neutral delivery contract for interactive CLI
  harnesses. Codex CLI has different terminal chrome than Claude Code, but an
  idle Codex composer must still be sendable and a Codex prompt menu must not be
  treated as an idle composer.

  Scenario: Codex composer is ready to receive input
    Given a Codex CLI composer pane
    When AGM checks whether the session can receive input
    Then delivery should be allowed
    And the detected session state should be "ready"

  Scenario: Codex trust prompt is not treated as ready
    Given a Codex CLI trust prompt
    When AGM checks whether the session can receive input
    Then delivery should be queued
