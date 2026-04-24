@implemented
Feature: Stall Detection
  As an AGM orchestrator
  I want stalled sessions to be detected and recovered
  So that the multi-agent system maintains forward progress

  Scenario: Permission prompt stalls are critical severity
    Given a session stuck in PERMISSION_PROMPT state
    When stall detection runs
    Then the stall event severity should be "critical"
    And the stall type should be "permission_prompt"

  Scenario: Error patterns are normalized before counting
    Given error messages with varying paths and line numbers
    When errors are normalized
    Then equivalent errors should be grouped together

  Scenario: Stall detector uses SLO contract thresholds
    Given a stall detector initialized from contracts
    Then the permission timeout should be "5m"
    And the no-commit timeout should be "15m"
    And the error repeat threshold should be 3

  Scenario: Only three stall types exist
    Given the stall detection system
    Then valid stall types should include "permission_prompt"
    And valid stall types should include "no_commit"
    And valid stall types should include "error_loop"
