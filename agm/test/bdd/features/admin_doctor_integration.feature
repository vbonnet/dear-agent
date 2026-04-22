Feature: Admin Doctor - Orphan Detection Integration
  As an AGM user
  I want orphan detection integrated into agm admin doctor
  So that health checks include session registry integrity

  Background:
    Given I have AGM installed
    And "agm admin doctor" command exists

  Scenario: Doctor detects orphaned sessions
    Given 2 orphaned sessions exist in the current workspace
    When I run "agm admin doctor"
    Then the output should include a check named "Orphaned conversations"
    And the check should show status "WARNING"
    And the check should report "Found 2 orphaned sessions"
    And the check should suggest "Run: agm admin find-orphans --auto-import"

  Scenario: Doctor shows healthy when no orphans
    Given all sessions have manifests
    When I run "agm admin doctor"
    Then the "Orphaned conversations" check should show status "OK"
    And the check should report "No orphaned sessions found"

  Scenario: Doctor integrates with existing checks
    Given "agm admin doctor" has existing health checks
    When I run "agm admin doctor"
    Then the output should include all existing checks
    And the "Orphaned conversations" check should be included
    And the check order should be logical

  Scenario: Doctor severity levels match orphan severity
    Given a critical orphaned session exists (current workspace, recent activity)
    When I run "agm admin doctor"
    Then the "Orphaned conversations" check should show severity "WARNING"
    And the overall doctor status should reflect the warning

  Scenario: Doctor filters orphans by workspace
    Given orphaned sessions exist in multiple workspaces
    And I am in workspace "oss"
    When I run "agm admin doctor"
    Then the orphan check should only report orphans in "oss" workspace
    And orphans from other workspaces should not be counted

  Scenario: Doctor provides actionable remediation
    Given 3 orphaned sessions exist
    When I run "agm admin doctor"
    Then the "Orphaned conversations" check output should show:
      """
      ⚠ Orphaned conversations
        Status: WARNING
        Found 3 orphaned sessions in workspace 'oss'

        Remediation:
          Run: agm admin find-orphans --workspace oss --auto-import

        Impact: Risk of lost work if sessions are not recovered
      """

  Scenario: Doctor lists orphaned UUIDs in verbose mode
    Given orphaned sessions "uuid-001", "uuid-002", "uuid-003" exist
    When I run "agm admin doctor --verbose"
    Then the "Orphaned conversations" check should list all orphaned UUIDs
    And each UUID should show its last activity timestamp

  Scenario: Doctor JSON output includes orphan data
    Given 2 orphaned sessions exist
    When I run "agm admin doctor --json"
    Then the JSON output should include:
      """
      {
        "checks": [
          {
            "name": "Orphaned conversations",
            "status": "WARNING",
            "message": "Found 2 orphaned sessions",
            "remediation": "Run: agm admin find-orphans --auto-import",
            "details": {
              "orphan_count": 2,
              "orphan_uuids": ["uuid-001", "uuid-002"]
            }
          }
        ]
      }
      """

  Scenario: Doctor handles orphan detection errors gracefully
    Given the history.jsonl file is corrupted
    When I run "agm admin doctor"
    Then the "Orphaned conversations" check should show status "ERROR"
    And the check should report "Failed to detect orphans: corrupted history.jsonl"
    And the overall doctor status should show degraded health

  Scenario: Doctor check execution time
    Given 100 sessions exist
    When I run "agm admin doctor"
    Then the "Orphaned conversations" check should complete in < 2 seconds
    And the doctor output should not show performance warnings

  Scenario: Doctor integrates with doctor's existing severity system
    Given doctor uses severity levels: CRITICAL, WARNING, INFO
    And an orphaned session is detected
    When I run "agm admin doctor"
    Then the orphan check should use severity "WARNING"
    And the doctor summary should aggregate all severities correctly

  Scenario: Doctor check can be disabled
    Given I have disabled orphan checks in AGM config
    When I run "agm admin doctor"
    Then the "Orphaned conversations" check should be skipped
    And the output should show "Orphaned conversations: SKIPPED (disabled)"

  Scenario: Doctor check respects --quick flag
    Given doctor has a --quick mode for fast checks
    When I run "agm admin doctor --quick"
    Then the orphan check should use fast detection (count only, no details)
    And the output should show orphan count without listing UUIDs

  Scenario: Doctor shows orphan age in check details
    Given an orphaned session with last activity "2024-01-15T10:00:00Z"
    And the current date is "2024-02-19T10:00:00Z"
    When I run "agm admin doctor --verbose"
    Then the orphan check should show "Stale orphan (35 days old)"
    And the remediation should emphasize urgency for old orphans

  Scenario: Doctor integrates orphan count into summary
    Given 2 orphaned sessions exist
    And 1 other warning exists (e.g., low disk space)
    When I run "agm admin doctor"
    Then the summary should show:
      """
      === AGM Health Summary ===
      Critical issues: 0
      Warnings: 2
        - Orphaned conversations (2 sessions)
        - Low disk space
      Info: 5 checks passed
      """

  Scenario: Doctor links to detailed orphan report
    Given orphaned sessions exist
    When I run "agm admin doctor"
    Then the orphan check should suggest "For details, run: agm admin find-orphans"
    And the output should include a direct link to the command
