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

  Scenario: Codex detached session receives startup prompt
    Given Codex CLI is available
    When AGM creates a detached Codex session with a startup prompt
    Then AGM should wait for the Codex composer
    And AGM should deliver the startup prompt even though the session is detached

  Scenario: Current harness session can be associated with AGM
    Given an existing tmux session running Codex CLI
    When /agm:agm-assoc runs in that session
    Then AGM should create or update a Dolt session record with harness "codex-cli"
    And AGM should create the ready-file signal

  Scenario: Session list fields can target session rows
    Given AGM has Codex session records in Dolt
    When an agent lists sessions as JSON with fields "name,status,harness,workspace,tags"
    Then the output should include a "sessions" array
    And each session row should include the requested fields
    And the output should not collapse to an empty object

  Scenario: Codex lifecycle commands work end to end
    Given a Codex CLI session created by AGM
    When AGM sends a message to the session
    And AGM resumes the session
    And AGM kills the session
    And AGM archives the stopped session
    Then Dolt should reflect the expected lifecycle transitions
