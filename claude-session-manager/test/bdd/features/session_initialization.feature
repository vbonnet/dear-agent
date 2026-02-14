Feature: Session Initialization
  As an AGM user
  I want new Claude sessions to be initialized automatically
  So that sessions are properly renamed and associated without manual intervention

  Background:
    Given I have AGM installed
    And I have Claude CLI installed

  Scenario: Successful session initialization with Claude
    Given no session named "test-init-success" exists
    When I run "agm session new test-init-success --agent=claude"
    Then the command should succeed within 90 seconds
    And a tmux session named "test-init-success" should exist
    And Claude should be running in the session
    And the session should be renamed to "test-init-success"
    And the session should be associated with AGM

  Scenario: Session initialization handles Claude startup delay
    Given no session named "test-init-slow" exists
    When I run "agm session new test-init-slow --agent=claude"
    Then the command should succeed within 90 seconds
    And Claude should start within 60 seconds
    And the session should be renamed to "test-init-slow"
    And the session should be associated with AGM

  Scenario: Session initialization timeout is handled gracefully
    Given no session named "test-init-timeout" exists
    And I have a mock agent that never starts
    When I run "agm session new test-init-timeout --agent=mock-agent"
    Then the command should complete within 120 seconds
    And I should see a warning about initialization timeout
    And the session "test-init-timeout" should still be attached
    And I should be able to manually run "/rename test-init-timeout"

  Scenario: Session initialization with trust prompt
    Given no session named "test-init-trust" exists
    And Claude will show a trust prompt on startup
    When I run "agm session new test-init-trust --agent=claude"
    Then the command should wait for user input
    When I answer "Yes, proceed" to the trust prompt
    Then the session should continue initialization
    And the session should be renamed to "test-init-trust"
    And the session should be associated with AGM

  Scenario: Multiple sessions can be initialized in parallel
    Given no session named "test-init-parallel-1" exists
    And no session named "test-init-parallel-2" exists
    When I run "agm session new test-init-parallel-1 --agent=claude" in the background
    And I run "agm session new test-init-parallel-2 --agent=claude" in the background
    Then both commands should succeed within 90 seconds
    And both sessions should be properly initialized
    And there should be no race conditions

  Scenario: Session initialization survives network interruption
    Given no session named "test-init-network" exists
    When I run "agm session new test-init-network --agent=claude"
    And there is a brief network interruption during initialization
    Then the initialization should complete successfully
    And the session should be renamed to "test-init-network"
    And the session should be associated with AGM
