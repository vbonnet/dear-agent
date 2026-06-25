Feature: Harness parity
  AGM should use one harness-neutral delivery contract for interactive CLI
  harnesses. Codex CLI and AGY have different terminal chrome than Claude
  Code, but their idle prompts must still be sendable and their trust/menu
  prompts must not be treated as ready.

  Scenario: Codex composer is ready to receive input
    Given a Codex CLI composer pane
    When AGM checks whether the session can receive input
    Then delivery should be allowed
    And the detected session state should be "ready"

  Scenario: Codex trust prompt is not treated as ready
    Given a Codex CLI trust prompt
    When AGM checks whether the session can receive input
    Then delivery should be queued

  Scenario: AGY prompt is ready to receive input
    Given an AGY ready prompt
    When AGM checks whether the session can receive input
    Then delivery should be allowed
    And the detected session state should be "ready"

  Scenario: AGY trust prompt is not treated as ready
    Given an AGY trust prompt
    When AGM checks whether the session can receive input
    Then delivery should be queued

  Scenario: Codex detached session receives startup prompt
    Given Codex CLI is available
    When AGM creates a detached Codex session with a startup prompt
    Then AGM should wait for the Codex composer
    And AGM should deliver the startup prompt even though the session is detached

  Scenario: AGY detached session receives startup prompt
    Given AGY is available
    When AGM creates a detached AGY session with a startup prompt
    Then AGM should wait for the AGY prompt
    And AGM should deliver the startup prompt even though the session is detached

  Scenario: Current harness session can be associated with AGM
    Given an existing tmux session running Codex CLI
    When /agm:agm-assoc runs in that session
    Then AGM should create or update a Dolt session record with harness "codex-cli"
    And AGM should create the ready-file signal

  Scenario: Current AGY session can be associated with AGM
    Given an existing tmux session running AGY
    When /agm:agm-assoc runs in that session
    Then AGM should create or update a Dolt session record with harness "agy"
    And AGM should create the ready-file signal

  Scenario: Orphaned Codex conversation can be imported and resumed
    Given a Codex saved session exists outside AGM
    When AGM imports the Codex session UUID with harness "codex-cli"
    Then AGM should create or update a Dolt session record with harness "codex-cli"
    And the record should preserve the Codex session UUID
    And AGM should launch a tmux pane that resumes the Codex conversation

  Scenario: Orphaned AGY conversation can be imported and resumed
    Given an AGY saved conversation exists outside AGM
    When AGM imports the AGY conversation ID with harness "agy"
    Then AGM should create or update a Dolt session record with harness "agy"
    And the record should preserve the AGY conversation ID
    And AGM should launch a tmux pane that resumes the AGY conversation

  Scenario: AGY auto permission mode is preserved on resume
    Given an imported AGY session with permission mode "auto"
    When AGM resumes the session
    Then AGM should launch a tmux pane that resumes the AGY conversation
    And the AGY resume command should include "--dangerously-skip-permissions"

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
    And the matching Codex saved session should be archived
