@implemented
Feature: Trust Protocol
  As an AGM operator
  I want trust scores to accurately reflect agent reliability
  So that orchestrator decisions about delegation are well-informed

  Scenario: Trust score is always clamped to [0, 100]
    Given a session "test-clamp" with no trust history
    When I record 20 "false_completion" events for "test-clamp"
    Then the trust score for "test-clamp" should be 0
    And the score should never be negative

  Scenario: Base score for new sessions is 50
    Given a session "test-base" with no trust history
    Then the trust score for "test-base" should be 50

  Scenario: Trust events are append-only
    Given a session "test-append" with no trust history
    When I record a "success" event for "test-append"
    And I record a "stall" event for "test-append"
    Then the trust history for "test-append" should have 2 events
    And the events should be in chronological order

  Scenario: gc_archived has zero score impact
    Given a session "test-gc" with no trust history
    When I record a "gc_archived" event for "test-gc"
    Then the trust score for "test-gc" should be 50

  Scenario: false_completion is the heaviest penalty
    Given a session "test-penalty" with no trust history
    When I record a "false_completion" event for "test-penalty"
    Then the trust score for "test-penalty" should be 35

  Scenario: Empty session name is rejected
    When I attempt to record a trust event with empty session name
    Then an invalid input error should be returned

  Scenario: Invalid event type is rejected
    When I attempt to record a trust event with type "invalid_type" for session "test-invalid"
    Then an invalid input error should be returned
    And the error should list valid event types
