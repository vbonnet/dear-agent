Feature: Session Search
  As an AGM user
  I want to search conversation content for keywords or patterns
  So that I can find sessions discussing specific topics

  Background:
    Given I have AGM installed
    And I have Claude history.jsonl with conversation content

  Scenario: Simple keyword search
    Given session "session-001" contains the text "implement authentication"
    And session "session-002" contains the text "refactor database schema"
    And session "session-003" contains the text "authentication middleware"
    When I run "agm session search authentication"
    Then the output should list sessions "session-001" and "session-003"
    And the output should not list session "session-002"

  Scenario: Case-insensitive search by default
    Given session "session-001" contains "Authentication Setup"
    And session "session-002" contains "AUTHENTICATION FLOW"
    And session "session-003" contains "authentication helpers"
    When I run "agm session search authentication"
    Then the output should list all 3 sessions

  Scenario: Case-sensitive search
    Given session "session-001" contains "Authentication"
    And session "session-002" contains "authentication"
    When I run "agm session search authentication --case-sensitive"
    Then the output should only list session "session-002"
    And the output should not list session "session-001"

  Scenario: Regular expression search
    Given session "session-001" contains "test-001.go"
    And session "session-002" contains "test-002.go"
    And session "session-003" contains "testing.md"
    When I run "agm session search 'test-\d+\.go' --regex"
    Then the output should list sessions "session-001" and "session-002"
    And the output should not list session "session-003"

  Scenario: Display match count
    Given session "session-001" contains "error" 5 times
    And session "session-002" contains "error" 2 times
    When I run "agm session search error"
    Then the output should show:
      | Session     | Matches |
      | session-001 | 5       |
      | session-002 | 2       |
    And sessions should be sorted by match count descending

  Scenario: Show context snippets
    Given session "session-001" contains:
      """
      We need to implement authentication using JWT tokens.
      The authentication flow should handle OAuth providers.
      """
    When I run "agm session search authentication"
    Then the output should include context snippets
    And the snippet should highlight "authentication" in the text
    And the snippet should show surrounding context

  Scenario: Filter by workspace
    Given session "session-001" in workspace "oss" contains "authentication"
    And session "session-002" in workspace "acme" contains "authentication"
    When I run "agm session search authentication --workspace oss"
    Then the output should only list session "session-001"

  Scenario: No matches found
    Given no sessions contain the text "nonexistent-term"
    When I run "agm session search nonexistent-term"
    Then the output should show "No sessions found matching 'nonexistent-term'"
    And the exit code should be 0

  Scenario: Search across multiple files in conversation
    Given session "session-001" has files:
      | File        | Content                  |
      | file1.md    | Contains authentication  |
      | file2.go    | Contains database logic  |
    When I run "agm session search authentication"
    Then the output should list session "session-001"
    And the context should show the match is from "file1.md"

  Scenario: Limit number of results
    Given 50 sessions contain "error"
    When I run "agm session search error --limit 10"
    Then the output should show exactly 10 sessions
    And the output should indicate "Showing 10 of 50 matches"

  Scenario: JSON output mode
    Given session "session-001" contains "authentication" 3 times
    When I run "agm session search authentication --json"
    Then the output should be valid JSON
    And the JSON should have structure:
      """
      {
        "query": "authentication",
        "total_matches": 1,
        "sessions": [
          {
            "uuid": "session-001",
            "name": "session-name",
            "workspace": "oss",
            "match_count": 3,
            "snippets": [
              {
                "text": "...context...authentication...context...",
                "file": "conversation.jsonl",
                "line": 42
              }
            ]
          }
        ]
      }
      """

  Scenario: Search in orphaned sessions
    Given an orphaned session "orphan-001" contains "important work"
    When I run "agm session search 'important work'"
    Then the output should list session "orphan-001"
    And the session name should be "<orphaned>"
    And the output should include a note about the orphaned session

  Scenario: Exclude archived sessions
    Given an archived session "archived-001" contains "authentication"
    And an active session "active-001" contains "authentication"
    When I run "agm session search authentication"
    Then the output should only list session "active-001"
    And the output should not list session "archived-001"

  Scenario: Include archived sessions with flag
    Given an archived session "archived-001" contains "authentication"
    When I run "agm session search authentication --include-archived"
    Then the output should list session "archived-001"
    And the session should be marked as "[ARCHIVED]"

  Scenario: Search with date range
    Given session "session-001" from "2024-02-01" contains "authentication"
    And session "session-002" from "2024-02-15" contains "authentication"
    When I run "agm session search authentication --since 2024-02-10"
    Then the output should only list session "session-002"

  Scenario: Multi-word phrase search
    Given session "session-001" contains "implement OAuth authentication"
    And session "session-002" contains "implement" and "OAuth" in different sentences
    When I run "agm session search 'implement OAuth'"
    Then the output should list both sessions
    But session "session-001" should have higher relevance score

  Scenario: Handle corrupted conversation files
    Given session "session-001" has a corrupted conversation file
    And session "session-002" has valid conversation containing "test"
    When I run "agm session search test"
    Then the command should not fail
    And the output should contain a warning about skipped corrupted session
    And session "session-002" should still be listed

  Scenario: Search with stemming
    Given session "session-001" contains "authenticate"
    And session "session-002" contains "authentication"
    And session "session-003" contains "authenticating"
    When I run "agm session search auth --stemming"
    Then the output should list all 3 sessions

  Scenario: Discover sessions across multiple configured workspaces
    Given AGM config with workspaces "personal" and "oss"
    And workspace "personal" has session "my-session"
    And workspace "oss" has session "oss-session"
    When I list sessions with "--all-workspaces"
    Then I should see session "my-session" in workspace "personal"
    And I should see session "oss-session" in workspace "oss"
