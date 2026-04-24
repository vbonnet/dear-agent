Feature: Admin Audit
  As an AGM user
  I want a comprehensive health report of all sessions across workspaces
  So that I can identify and fix session integrity issues

  Background:
    Given I have AGM installed
    And I have multiple sessions across different workspaces

  Scenario: Run full audit across all workspaces
    Given the following session states:
      | Type               | Count |
      | Orphaned           | 2     |
      | Corrupted manifest | 1     |
      | Missing tmux       | 1     |
      | Stale (>30 days)   | 1     |
      | Duplicate UUIDs    | 2     |
      | Healthy            | 3     |
    When I run "agm admin audit"
    Then the output should be grouped by severity
    And the CRITICAL section should contain 3 issues
    And the WARNING section should contain 2 issues
    And the INFO section should contain 3 healthy sessions

  Scenario: Audit grouped by severity levels
    Given sessions with various issues exist
    When I run "agm admin audit"
    Then the output should have sections in order:
      | Section  |
      | CRITICAL |
      | WARNING  |
      | INFO     |
    And each section should have a summary count

  Scenario: Critical - Orphaned conversations
    Given 2 orphaned sessions exist
    When I run "agm admin audit"
    Then the CRITICAL section should contain "2 orphaned conversations"
    And the output should list each orphaned UUID
    And the output should suggest "Run: agm admin find-orphans --auto-import"

  Scenario: Critical - Corrupted manifests
    Given a corrupted manifest exists for session "mock-corrupted-001"
    When I run "agm admin audit"
    Then the CRITICAL section should contain "Corrupted manifest: mock-corrupted-001"
    And the error details should show the YAML parse error

  Scenario: Critical - Duplicate UUIDs
    Given two sessions have the same Claude UUID "duplicate-uuid-dddddddd-eeee-ffff-0000-111111111111"
    When I run "agm admin audit"
    Then the CRITICAL section should contain "Duplicate UUID"
    And the output should list both session IDs with the duplicate UUID

  Scenario: Warning - Missing tmux sessions
    Given a session has manifest but tmux session does not exist
    When I run "agm admin audit"
    Then the WARNING section should contain "Missing tmux session"
    And the output should show the expected tmux session name

  Scenario: Warning - Stale sessions
    Given a session with last activity "2023-12-15T10:00:00Z"
    And the current date is "2024-02-19T10:00:00Z"
    When I run "agm admin audit"
    Then the WARNING section should contain "Stale session (inactive 66 days)"
    And the output should suggest archiving the session

  Scenario: Filter by severity level
    Given sessions with CRITICAL, WARNING, and INFO issues exist
    When I run "agm admin audit --severity CRITICAL"
    Then the output should only show CRITICAL issues
    And the output should not contain WARNING or INFO sections

  Scenario: Filter by workspace
    Given sessions in "oss", "acme", and "research" workspaces
    When I run "agm admin audit --workspace oss"
    Then the output should only include sessions from "oss" workspace
    And the summary should show "Workspace: oss"

  Scenario: JSON output mode
    Given sessions with various issues exist
    When I run "agm admin audit --json"
    Then the output should be valid JSON
    And the JSON should have keys "critical", "warning", "info"
    And each issue should have fields "type", "session_id", "description", "remediation"

  Scenario: Actionable recommendations
    Given an orphaned session exists
    When I run "agm admin audit"
    Then each issue should have a "Remediation" field
    And the orphan issue should suggest "agm admin find-orphans --auto-import"
    And the stale session should suggest "agm session archive <session-id>"

  Scenario: Healthy workspace
    Given all sessions are healthy
    When I run "agm admin audit"
    Then the output should contain "No critical issues found"
    And the output should contain "No warnings"
    And the INFO section should list all healthy sessions

  Scenario: Exclude archived sessions from checks
    Given an archived session exists
    When I run "agm admin audit"
    Then the archived session should not appear in any issues
    And the archived session should not be counted in stale checks

  Scenario: Check manifest-tmux consistency
    Given a session manifest has tmux_session_name "agm-oss-test"
    But the actual tmux session is named "agm-oss-different"
    When I run "agm admin audit"
    Then the WARNING section should contain "Tmux session name mismatch"
    And the output should show expected vs actual names

  Scenario: Summary statistics
    Given 10 total sessions with 3 issues
    When I run "agm admin audit"
    Then the output should end with a summary:
      """
      === Summary ===
      Total sessions: 10
      Critical issues: 1
      Warnings: 2
      Healthy: 7
      """

  Scenario: Audit current session only
    Given I am in an AGM session
    When I run "agm admin audit --current-session"
    Then the audit should only check the current session
    And the output should show pass/fail for current session

  Scenario: Gate 9 integration - VerificationResult JSON
    Given I am in an AGM session
    When I run "agm admin audit --current-session --json"
    Then the output should be a VerificationResult JSON
    And the JSON should have fields:
      | Field         |
      | gate_name     |
      | passed        |
      | severity      |
      | violations    |
      | message       |
      | remediation   |
