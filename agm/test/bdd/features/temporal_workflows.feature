Feature: Temporal Workflows
  As an AGM developer
  I want Temporal workflows to manage session lifecycle and monitoring
  So that sessions can be managed in a distributed, fault-tolerant manner

  Background:
    Given Temporal test server is running
    And Temporal worker is registered with workflows

  Scenario: SessionWorkflow manages session lifecycle
    Given a SessionWorkflow is started with session ID "test-session-001"
    And the workflow has session name "my-test-session"
    And the workflow has agent "claude"
    And the workflow has working directory "/tmp/test"
    When I query the workflow for session state
    Then the session state should be "active"
    And the session ID should be "test-session-001"
    And the session name should be "my-test-session"

  Scenario: SessionWorkflow handles state transitions
    Given a SessionWorkflow is running for session "test-session-002"
    When I send "stop" signal to the workflow
    Then the session state should transition to "stopped"
    When I send "activate" signal to the workflow
    Then the session state should transition to "active"
    When I send "archive" signal to the workflow
    Then the session state should transition to "archived"
    And the workflow should complete successfully

  Scenario: SessionWorkflow tracks attached clients
    Given a SessionWorkflow is running for session "test-session-003"
    And the session has 0 attached clients
    When I send "attach" signal to the workflow
    Then the session should have 1 attached client
    When I send "attach" signal to the workflow
    Then the session should have 2 attached clients
    When I send "detach" signal to the workflow
    Then the session should have 1 attached client

  Scenario: MonitorWorkflow detects escalation patterns
    Given a MonitorWorkflow is started with session ID "test-session-004"
    And the workflow has monitoring interval "1s"
    And the workflow has escalation rule:
      | name          | pattern | severity | notifyAfter |
      | Error Pattern | ERROR   | high     | 1           |
    When the session output contains "ERROR: Critical failure"
    And I wait for monitoring check
    Then an escalation should be triggered
    And the escalation severity should be "high"

  Scenario: MonitorWorkflow handles multiple escalation rules
    Given a MonitorWorkflow is started with session ID "test-session-005"
    And the workflow has escalation rules:
      | name            | pattern | severity | notifyAfter |
      | Error Pattern   | ERROR   | high     | 2           |
      | Warning Pattern | WARN    | medium   | 5           |
    When the session output contains "ERROR: First error"
    And I wait for monitoring check
    Then no escalation should be triggered
    When the session output contains "ERROR: Second error"
    And I wait for monitoring check
    Then an escalation should be triggered for "Error Pattern"

  Scenario: MonitorWorkflow can be stopped and restarted
    Given a MonitorWorkflow is running for session "test-session-006"
    And the workflow is monitoring
    When I send "stopMonitoring" signal to the workflow
    Then the workflow monitoring state should be false
    When I send "startMonitoring" signal to the workflow
    Then the workflow monitoring state should be true

  Scenario: EscalationWorkflow sends notifications to configured channels
    Given an EscalationWorkflow is started for session "test-session-007"
    And the workflow has severity "high"
    And the workflow has notification channels:
      | type    | target              | priority |
      | log     | escalation-high     | 0        |
      | webhook | http://example.com  | 1        |
    When the workflow executes
    Then notifications should be sent to 2 channels
    And the escalation status should be "completed"

  Scenario: EscalationWorkflow handles critical severity with fallback
    Given an EscalationWorkflow is started for session "test-session-008"
    And the workflow has severity "critical"
    And the workflow has notification channel that will fail
    When the workflow executes
    Then a fallback notification should be sent
    And the escalation should not fail completely

  Scenario: EscalationWorkflow retries based on severity
    Given an EscalationWorkflow is started for session "test-session-009"
    And the workflow has severity "critical"
    And the notification activity will fail 2 times then succeed
    When the workflow executes
    Then the notification should be retried
    And the escalation status should be "completed"

  Scenario: SessionWorkflow creates session via activity
    Given a SessionWorkflow is started with session ID "test-session-010"
    When the workflow initializes
    Then the "CreateSessionActivity" should be executed
    And the session should transition to "active" state

  Scenario: MonitorWorkflow starts child EscalationWorkflow
    Given a MonitorWorkflow is running for session "test-session-011"
    And the workflow has escalation rule with notifyAfter threshold 1
    When the session output matches the escalation pattern
    And I wait for monitoring check
    Then a child EscalationWorkflow should be started
    And the child workflow should complete successfully

  Scenario Outline: EscalationWorkflow uses correct retry policy for severity
    Given an EscalationWorkflow is started with severity "<severity>"
    When I query the workflow for escalation state
    Then the retry policy should have maximum attempts of <maxAttempts>
    And the initial retry interval should be approximately <initialInterval>

    Examples:
      | severity | maxAttempts | initialInterval |
      | critical | 5           | 1s              |
      | high     | 3           | 2s              |
      | medium   | 2           | 5s              |
      | low      | 2           | 5s              |

  Scenario: Multiple workflows can run concurrently for different sessions
    Given a SessionWorkflow is started for session "session-A"
    And a SessionWorkflow is started for session "session-B"
    And a MonitorWorkflow is started for session "session-A"
    And a MonitorWorkflow is started for session "session-B"
    When I send "stop" signal to session "session-A" workflow
    Then session "session-A" state should be "stopped"
    And session "session-B" state should be "active"

  Scenario: SessionWorkflow query handler provides current state
    Given a SessionWorkflow is running for session "test-session-012"
    And 2 clients are attached to the session
    When I query the workflow for session state
    Then the query should return current state without blocking
    And the attached clients count should be 2
    And the state should include timestamps

  Scenario: MonitorWorkflow updates escalation rules dynamically
    Given a MonitorWorkflow is running for session "test-session-013"
    And the workflow has initial escalation rules
    When I send "updateRules" signal with new rules:
      | name        | pattern  | severity | notifyAfter |
      | New Pattern | CRITICAL | critical | 1           |
    Then the workflow should use the new escalation rules
    And the match counters should be reset

  Scenario: EscalationWorkflow stores escalation record
    Given an EscalationWorkflow is started for session "test-session-014"
    When the workflow completes successfully
    Then the "StoreEscalationRecordActivity" should be executed
    And the escalation record should contain notification results
    And the escalation record should have completion timestamp

  Scenario: SessionWorkflow handles activity failures gracefully
    Given a SessionWorkflow is started with session ID "test-session-015"
    And the "CreateSessionActivity" will fail
    When the workflow initializes
    Then the workflow should retry the activity
    And the workflow should fail if retries are exhausted

  Scenario: MonitorWorkflow gracefully shuts down when stopped
    Given a MonitorWorkflow is running for session "test-session-016"
    When I send "stopMonitoring" signal twice
    Then the workflow should complete gracefully
    And no errors should be logged
