Feature: Session Association
  As an AGM user
  I want reliable session association with Claude UUID detection
  So that my sessions are properly tracked and manageable

  Background:
    Given I have AGM installed
    And the AGM state directory is "~/.agm"

  Scenario: Successful ready-file creation and detection
    Given a Claude session "test-assoc-1" is starting
    When the association process creates a ready-file
    And I wait for the ready-file with timeout "5s"
    Then the ready-file should be detected within timeout
    And the ready-file should contain status "ready"
    And the session UUID should be populated in the manifest
    And the ready-file should be cleaned up

  Scenario: Ready-file timeout handling
    Given a Claude session "test-timeout" is starting
    When I wait for the ready-file with timeout "1s"
    But no ready-file is created
    Then the wait should timeout after "1s"
    And an error should be returned with message "timeout waiting for ready-file"
    And the session should still be usable without UUID

  Scenario: Ready-file contains crash status
    Given a Claude session "test-crash" is starting
    When the association process creates a ready-file with status "crashed"
    And I wait for the ready-file with timeout "5s"
    Then the ready-file should be detected within timeout
    And the ready-file should contain status "crashed"
    And an error should be returned with message "Claude crashed during startup"
    And the ready-file should be cleaned up

  Scenario: Ready-file created in correct directory
    Given a Claude session "test-dir" is starting
    And the AGM state directory is "~/.agm"
    When the association process creates a ready-file
    Then the ready-file path should be "~/.agm/ready-test-dir"
    And the ready-file should exist at the expected path
    And the ready-file should be readable

  Scenario: Stale ready-file cleanup before new session
    Given a Claude session "test-stale" exists with a stale ready-file
    And the stale ready-file is older than "10m"
    When a new session "test-stale" is created
    Then the stale ready-file should be removed before watching
    And the new ready-file should be detected correctly

  Scenario: Race condition - ready-file exists before watch starts
    Given a Claude session "test-race" is starting
    And a ready-file already exists for "test-race"
    When I start waiting for the ready-file
    Then the pre-existing ready-file should be detected immediately
    And no timeout should occur
    And the ready-file should be cleaned up

  Scenario: Async event-driven ready-file detection
    Given a Claude session "test-async" is starting
    When I start watching for ready-file events asynchronously
    And the ready-file is created "500ms" after watch starts
    Then the fsnotify CREATE event should be detected
    And the ready-file should be detected before timeout
    And the watch should complete in less than "2s"

  Scenario: Ready-file validation after creation
    Given a Claude session "test-validation" is starting
    When the association process creates a ready-file
    Then the ready-file should exist
    And the ready-file should be readable
    And the ready-file should contain valid JSON
    And the ready-file JSON should have field "status"
    And the ready-file JSON should have field "ready_at"
    And the ready-file JSON should have field "session_name"

  Scenario: Graceful degradation - session usable without ready-file
    Given a Claude session "test-degradation" is starting
    When the ready-file creation fails
    Then the session creation should not fail
    And the session should be in "active" state
    And the UUID field should be empty in manifest
    And a warning should be logged about association failure

  Scenario: Multiple concurrent sessions isolated ready-files
    Given Claude sessions "session-A" and "session-B" are starting concurrently
    When ready-files are created for both sessions
    Then "session-A" should only detect its own ready-file
    And "session-B" should only detect its own ready-file
    And the ready-files should have different session names
    And both sessions should complete association successfully
