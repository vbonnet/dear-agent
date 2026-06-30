Feature: Harness parity
  AGM should use one harness-neutral delivery contract for interactive CLI
  harnesses. Claude Code is the reference implementation. Codex CLI, AGY, and
  OpenCode have different terminal chrome and control surfaces than Claude
  Code, but their idle prompts must still be sendable and their trust/menu
  prompts must not be treated as ready. Gemini CLI is deprecated compatibility
  and is not part of active parity enforcement.

  Scenario Outline: Active parity harnesses are canonical
    Given harness "<harness>" is configured
    When AGM validates active parity support
    Then harness "<harness>" should be active for parity
    And harness "<harness>" should not be deprecated

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |

  Scenario: Gemini CLI is deprecated compatibility
    Given harness "gemini-cli" is configured
    When AGM validates active parity support
    Then harness "gemini-cli" should be deprecated

  Scenario Outline: Supported model families have default routes
    Given model family "<family>" is configured
    When AGM validates model family parity support
    Then model family "<family>" should be supported
    And model family "<family>" should have a default model route

    Examples:
      | family    |
      | anthropic |
      | openai    |
      | gemini    |
      | glm       |
      | deepseek  |
      | nemotron  |
      | qwen      |

  Scenario Outline: Active harness model changes use the shared registry
    Given harness "<harness>" is configured
    When AGM resolves a model change for harness "<harness>" with model "<model>"
    Then the model change should use tmux command "/model"
    And the resolved model should not be empty

    Examples:
      | harness      | model     |
      | claude-code  | sonnet    |
      | codex-cli    | 5.4-mini  |
      | agy          | 2.5-flash |
      | opencode-cli | glm-5.2   |
      | opencode-cli | deepseek-v4 |
      | opencode-cli | nemotron  |
      | opencode-cli | qwen      |

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

  Scenario: Codex detached startup clears first-run trust before delivery
    Given Codex CLI is available
    And a Codex CLI trust prompt
    When AGM creates a detached Codex session with a startup prompt
    Then AGM should auto-accept the Codex trust prompt before prompt delivery
    And AGM should wait for the Codex composer

  Scenario: Codex send safety is harness-specific
    Given Codex CLI is available
    And a Codex CLI composer pane
    When AGM runs send safety for the configured harness
    Then send safety should not require a Claude process

  Scenario: AGY detached session receives startup prompt
    Given AGY is available
    When AGM creates a detached AGY session with a startup prompt
    Then AGM should wait for the AGY prompt
    And AGM should deliver the startup prompt even though the session is detached

  Scenario: AGY detached startup clears first-run trust before delivery
    Given AGY is available
    And an AGY trust prompt
    When AGM creates a detached AGY session with a startup prompt
    Then AGM should auto-accept the AGY trust prompt before prompt delivery
    And AGM should wait for the AGY prompt

  Scenario: AGY send safety is harness-specific
    Given AGY is available
    And an AGY ready prompt
    When AGM runs send safety for the configured harness
    Then send safety should not require a Claude process

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
