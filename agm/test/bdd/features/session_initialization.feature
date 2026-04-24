Feature: Session Initialization
  As an AGM user
  I want new Claude sessions to be initialized automatically
  So that sessions are properly renamed and associated without manual intervention

  Background:
    Given I have AGM installed
    And I have Claude CLI installed

  Scenario: Successful session initialization with Claude
    Given no session named "test-init-success" exists
    When I run "agm session new test-init-success --harness claude-code"
    Then the command should succeed within 90 seconds
    And a tmux session named "test-init-success" should exist
    And Claude should be running in the session
    And the session should be renamed to "test-init-success"
    And the session should be associated with AGM

  Scenario: Session initialization handles Claude startup delay
    Given no session named "test-init-slow" exists
    When I run "agm session new test-init-slow --harness claude-code"
    Then the command should succeed within 90 seconds
    And Claude should start within 60 seconds
    And the session should be renamed to "test-init-slow"
    And the session should be associated with AGM

  Scenario: Session initialization timeout is handled gracefully
    Given no session named "test-init-timeout" exists
    And I have a mock agent that never starts
    When I run "agm session new test-init-timeout --harness mock-agent"
    Then the command should complete within 120 seconds
    And I should see a warning about initialization timeout
    And the session "test-init-timeout" should still be attached
    And I should be able to manually run "/rename test-init-timeout"

  Scenario: Session initialization with trust prompt
    Given no session named "test-init-trust" exists
    And Claude will show a trust prompt on startup
    When I run "agm session new test-init-trust --harness claude-code"
    Then the command should wait for user input
    When I answer "Yes, proceed" to the trust prompt
    Then the session should continue initialization
    And the session should be renamed to "test-init-trust"
    And the session should be associated with AGM

  Scenario: Multiple sessions can be initialized in parallel
    Given no session named "test-init-parallel-1" exists
    And no session named "test-init-parallel-2" exists
    When I run "agm session new test-init-parallel-1 --harness claude-code" in the background
    And I run "agm session new test-init-parallel-2 --harness claude-code" in the background
    Then both commands should succeed within 90 seconds
    And both sessions should be properly initialized
    And there should be no race conditions

  Scenario: Session initialization survives network interruption
    Given no session named "test-init-network" exists
    When I run "agm session new test-init-network --harness claude-code"
    And there is a brief network interruption during initialization
    Then the initialization should complete successfully
    And the session should be renamed to "test-init-network"
    And the session should be associated with AGM

  # Regression tests for double-lock and command queueing bugs

  Scenario: Initialization does not cause double-lock errors
    Given no session named "test-no-double-lock" exists
    When I run "agm session new test-no-double-lock --harness claude-code"
    Then the command should not produce "lock already held" errors
    And the command should not produce "tmux lock" errors
    And the session should initialize successfully

  Scenario: Commands execute on separate lines in detached mode
    Given no session named "test-separate-commands" exists
    When I run "agm session new test-separate-commands --harness claude-code --detached"
    And I wait for initialization to complete
    And I capture the tmux pane content
    Then "/rename test-separate-commands" should be on one line
    And "/agm:agm-assoc test-separate-commands" should be on a different line
    And "/rename" should execute before "/agm:agm-assoc"

  Scenario: Sufficient delay between sequential commands
    Given no session named "test-command-timing" exists
    When I run "agm session new test-command-timing --harness claude-code --detached"
    Then the initialization should take at least 6 seconds
    And the "/rename" command should execute completely
    And the "/agm:agm-assoc" command should execute after "/rename" completes

  Scenario: SendCommandLiteral uses correct tmux send-keys format
    Given a test tmux session exists
    When I send a command with special characters using SendCommandLiteral
    Then the special characters should be interpreted literally
    And the command should not be interpreted by the shell
    And the command should execute with the -l flag

  Scenario: Detached sessions initialize without user interaction
    Given no session named "test-detached-init" exists
    When I run "agm session new test-detached-init --harness claude-code --detached"
    Then I should not need to attach to the session
    And the initialization should complete automatically within 60 seconds
    And both "/rename" and "/agm:agm-assoc" should execute
    And the ready-file signal should be created
