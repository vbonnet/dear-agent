Feature: Session Import
  As an AGM user
  I want to import an orphaned conversation by UUID
  So that I can resume work on abandoned sessions

  Background:
    Given I have AGM installed
    And I have Claude history.jsonl

  Scenario: Import orphaned session with auto-inferred metadata
    Given an orphaned session "370980e1-e16c-48a1-9d17-caca0d3910ba" exists in history
    And the session project is "~/src/ws/oss"
    And the session last activity was "2024-02-19T10:00:00Z"
    When I run "agm session import 370980e1-e16c-48a1-9d17-caca0d3910ba"
    Then a manifest should be created
    And the manifest should have:
      | Field              | Value                                      |
      | schema_version     | 2.0                                        |
      | claude.uuid        | 370980e1-e16c-48a1-9d17-caca0d3910ba       |
      | context.project    | ~/src/ws/oss                      |
      | workspace          | oss                                        |
    And the tmux session name should be sanitized
    And the output should contain "Successfully imported session 370980e1-e16c-48a1-9d17-caca0d3910ba"

  Scenario: Import with custom session name
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890 --name 'My Custom Session'"
    Then the manifest should have name "My Custom Session"
    And the tmux session name should be "agm-oss-my-custom-session"

  Scenario: Import with explicit workspace
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    And the session project is "/tmp/unknown-project"
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890 --workspace acme"
    Then the manifest should have workspace "acme"

  Scenario: Prevent duplicate import
    Given a session "tracked-session-uuid-001" with an existing manifest
    When I run "agm session import tracked-session-uuid-001"
    Then the exit code should be 1
    And the error should contain "Session already has a manifest"
    And no new manifest should be created

  Scenario: Handle invalid UUID
    When I run "agm session import invalid-uuid-format"
    Then the exit code should be 1
    And the error should contain "Invalid UUID format"

  Scenario: Handle UUID not in history
    When I run "agm session import aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    Then the exit code should be 1
    And the error should contain "UUID not found in history.jsonl"
    And the error should suggest checking the UUID

  Scenario: Sanitize tmux session name with special characters
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890 --name 'Project: Test/Debug (v2)'"
    Then the tmux session name should be "agm-oss-project-test-debug-v2"
    And the manifest name should be "Project: Test/Debug (v2)"

  Scenario: Infer workspace from project path
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    And the session project is "~/src/ws/acme/project"
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890"
    Then the manifest should have workspace "acme"

  Scenario: Default to 'default' workspace when path inference fails
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    And the session project is "/tmp/unknown-location"
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890"
    Then the manifest should have workspace "default"

  Scenario: Create manifest with correct timestamps
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    And the session last activity was "2024-02-19T10:00:00Z"
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890"
    Then the manifest should have created_at matching current time
    And the manifest should have updated_at matching the last activity time

  Scenario: Auto-generate session ID
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890"
    Then the manifest should have a unique session_id
    And the session_id should not be the Claude UUID

  Scenario: Dry-run mode
    Given an orphaned session "a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890" exists
    When I run "agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890 --dry-run"
    Then the output should show the manifest that would be created
    And no manifest file should be written
    And the output should contain "DRY RUN - no changes made"
