Feature: Admin Find Orphans
  As an AGM user
  I want to detect orphaned conversations across all workspaces
  So that I can recover abandoned work and prevent data loss

  Background:
    Given I have AGM installed
    And I have Claude history.jsonl with orphaned sessions
    And I have some tracked sessions with manifests

  Scenario: Detect orphaned sessions across all workspaces
    Given the following sessions exist:
      | UUID                                   | Workspace | Has Manifest |
      | 370980e1-e16c-48a1-9d17-caca0d3910ba   | oss       | no           |
      | a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890   | acme    | no           |
      | tracked-session-uuid-001               | oss       | yes          |
    When I run "agm admin find-orphans"
    Then the output should contain "Found 2 orphaned sessions"
    And the output should contain "370980e1-e16c-48a1-9d17-caca0d3910ba"
    And the output should contain "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890"
    And the output should not contain "tracked-session-uuid-001"

  Scenario: Filter orphans by workspace
    Given the following sessions exist:
      | UUID                                   | Workspace | Has Manifest |
      | 370980e1-e16c-48a1-9d17-caca0d3910ba   | oss       | no           |
      | a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890   | acme    | no           |
      | orphan-multi-workspace-001             | research  | no           |
    When I run "agm admin find-orphans --workspace oss"
    Then the output should contain "Found 1 orphaned session"
    And the output should contain "370980e1-e16c-48a1-9d17-caca0d3910ba"
    And the output should not contain "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890"

  Scenario: Display orphan details in table format
    Given an orphaned session "370980e1-e16c-48a1-9d17-caca0d3910ba" exists
    And the session last activity was "2024-02-19T10:00:00Z"
    And the session project is "~/src/ws/oss"
    When I run "agm admin find-orphans"
    Then the output should be formatted as a table
    And the table should have columns "UUID", "Workspace", "Project", "Last Activity", "Status"
    And the row for "370980e1-e16c-48a1-9d17-caca0d3910ba" should show status "ORPHANED"

  Scenario: Auto-import orphaned sessions
    Given an orphaned session "370980e1-e16c-48a1-9d17-caca0d3910ba" exists
    When I run "agm admin find-orphans --auto-import"
    Then I should be prompted "Import session 370980e1-e16c-48a1-9d17-caca0d3910ba? (y/n)"
    When I respond "y"
    Then a manifest should be created for "370980e1-e16c-48a1-9d17-caca0d3910ba"
    And the manifest should have a sanitized tmux session name
    And the output should contain "Successfully imported session 370980e1-e16c-48a1-9d17-caca0d3910ba"

  Scenario: Skip import when user declines
    Given an orphaned session "370980e1-e16c-48a1-9d17-caca0d3910ba" exists
    When I run "agm admin find-orphans --auto-import"
    And I respond "n" to the import prompt
    Then no manifest should be created for "370980e1-e16c-48a1-9d17-caca0d3910ba"
    And the output should contain "Skipped session 370980e1-e16c-48a1-9d17-caca0d3910ba"

  Scenario: Handle no orphaned sessions found
    Given all sessions have manifests
    When I run "agm admin find-orphans"
    Then the output should contain "No orphaned sessions found"
    And the exit code should be 0

  Scenario: Mark stale orphaned sessions
    Given an orphaned session "orphan-stale-session-001" exists
    And the session last activity was "2024-01-21T10:00:00Z"
    And the current date is "2024-02-19T10:00:00Z"
    When I run "agm admin find-orphans"
    Then the row for "orphan-stale-session-001" should show status "ORPHANED (STALE)"
    And the output should contain a warning about stale sessions

  Scenario: Handle corrupted history.jsonl during orphan detection
    Given the history.jsonl file has corrupted entries
    When I run "agm admin find-orphans"
    Then the command should not fail
    And the output should contain a warning about skipped corrupted entries
    And valid orphans should still be detected

  Scenario: JSON output mode
    Given an orphaned session "370980e1-e16c-48a1-9d17-caca0d3910ba" exists
    When I run "agm admin find-orphans --json"
    Then the output should be valid JSON
    And the JSON should contain an array of orphaned sessions
    And each session should have fields "uuid", "workspace", "project", "last_activity", "stale"

  Scenario: Error when history.jsonl not found
    Given the history.jsonl file does not exist
    When I run "agm admin find-orphans"
    Then the exit code should be 1
    And the error should contain "history.jsonl not found"
    And the error should suggest "Have you used Claude CLI before?"
