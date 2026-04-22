Feature: Admin Trace Files
  As an AGM user
  I want to find which sessions modified specific files
  So that I can understand file provenance and recover context

  Background:
    Given I have AGM installed
    And I have Claude history.jsonl with file modification records

  Scenario: Trace single file to sessions
    Given the file "~/src/ws/oss/README.md" was modified by session "session-file-edit-001"
    And the modifications occurred at:
      | Timestamp           |
      | 2024-02-19T08:00:00 |
      | 2024-02-19T11:20:00 |
    When I run "agm admin trace-files ~/src/ws/oss/README.md"
    Then the output should show:
      """
      File: ~/src/ws/oss/README.md
      Session: session-file-edit-001 (readme-updates)
        - 2024-02-19 08:00:00
        - 2024-02-19 11:20:00
      """

  Scenario: Trace multiple files
    Given the following file modifications:
      | File                                 | Session                | Timestamp           |
      | ~/src/ws/oss/README.md      | session-file-edit-001  | 2024-02-19T08:00:00 |
      | ~/src/ws/oss/src/main.go    | session-file-edit-002  | 2024-02-19T10:46:00 |
    When I run "agm admin trace-files ~/src/ws/oss/README.md ~/src/ws/oss/src/main.go"
    Then the output should show results for both files
    And each file should list its modifying sessions

  Scenario: File not found in history
    When I run "agm admin trace-files ~/src/ws/oss/never-modified.txt"
    Then the output should show:
      """
      File: ~/src/ws/oss/never-modified.txt
      No sessions found
      """
    And the exit code should be 0

  Scenario: Filter by date range
    Given "~/src/ws/oss/README.md" was modified at:
      | Timestamp           |
      | 2024-02-19T08:00:00 |
      | 2024-02-19T11:20:00 |
      | 2024-02-19T13:30:00 |
    When I run "agm admin trace-files ~/src/ws/oss/README.md --since 2024-02-19T10:00:00"
    Then the output should only show modifications after 10:00
    And the output should include timestamps 11:20 and 13:30
    And the output should not include timestamp 08:00

  Scenario: Multiple sessions modified same file
    Given "~/src/ws/oss/shared.go" was modified by:
      | Session                | Timestamp           |
      | session-file-edit-001  | 2024-02-19T08:00:00 |
      | session-file-edit-002  | 2024-02-19T10:00:00 |
      | session-file-edit-004  | 2024-02-19T12:00:00 |
    When I run "agm admin trace-files ~/src/ws/oss/shared.go"
    Then the output should show all 3 sessions
    And sessions should be ordered by first modification time

  Scenario: Table output format
    Given file modifications exist
    When I run "agm admin trace-files ~/src/ws/oss/README.md"
    Then the output should be formatted as a table
    And the table should have columns "File", "Session UUID", "Session Name", "Timestamp"

  Scenario: JSON output mode
    Given "~/src/ws/oss/README.md" was modified by session "session-file-edit-001"
    When I run "agm admin trace-files ~/src/ws/oss/README.md --json"
    Then the output should be valid JSON
    And the JSON should have structure:
      """
      {
        "files": [
          {
            "path": "~/src/ws/oss/README.md",
            "sessions": [
              {
                "uuid": "session-file-edit-001",
                "name": "readme-updates",
                "modifications": [
                  {"timestamp": "2024-02-19T08:00:00Z"},
                  {"timestamp": "2024-02-19T11:20:00Z"}
                ]
              }
            ]
          }
        ]
      }
      """

  Scenario: Handle corrupted history during trace
    Given the history.jsonl file has corrupted entries
    When I run "agm admin trace-files ~/src/ws/oss/README.md"
    Then the command should not fail
    And the output should contain a warning about skipped corrupted entries
    And valid file modifications should still be traced

  Scenario: Relative path handling
    Given I am in directory "~/src/ws/oss"
    When I run "agm admin trace-files README.md"
    Then the command should resolve the absolute path
    And trace the file "~/src/ws/oss/README.md"

  Scenario: Substring path matching
    Given the following files exist:
      | File                                        |
      | ~/src/ws/oss/internal/pkg/util.go  |
      | ~/src/ws/oss/internal/pkg/types.go |
    When I run "agm admin trace-files --pattern 'internal/pkg/*.go'"
    Then the output should show both files
    And each file should list its modifying sessions

  Scenario: Case-sensitive path matching
    Given "~/src/ws/oss/README.md" exists
    When I run "agm admin trace-files ~/src/ws/oss/readme.md"
    Then the output should show "No sessions found"
    And the output should suggest checking the file path

  Scenario: Orphaned session file modifications
    Given an orphaned session "orphan-001" modified "~/src/ws/oss/file.txt"
    And the session does not have a manifest
    When I run "agm admin trace-files ~/src/ws/oss/file.txt"
    Then the output should show session UUID "orphan-001"
    And the session name should be "<no manifest>"
    And the output should include a note about the orphaned session

  Scenario: Filter by workspace
    Given file modifications in multiple workspaces:
      | File                                 | Workspace |
      | ~/src/ws/oss/file1.txt      | oss       |
      | ~/src/ws/acme/file2.txt   | acme    |
    When I run "agm admin trace-files --workspace oss ~/src/ws/oss/file1.txt ~/src/ws/acme/file2.txt"
    Then the output should only show sessions from "oss" workspace
    And "~/src/ws/acme/file2.txt" should show "No sessions found (in workspace oss)"

  Scenario: Show session context in output
    Given "~/src/ws/oss/README.md" was modified by session "session-file-edit-001"
    And the session has purpose "Update documentation"
    When I run "agm admin trace-files ~/src/ws/oss/README.md --verbose"
    Then the output should include session purpose
    And the output should show the session project path
