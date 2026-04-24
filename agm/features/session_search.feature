Feature: Session Content Search
  As a developer using AGM
  I want to search conversation content for keywords or patterns
  So that I can find past sessions about specific topics

  Background:
    Given I have a sessions directory at "~/sessions"
    And I have Claude history at "~/.claude/history.jsonl"
    And the following sessions exist:
      | uuid           | name             | workspace | content                                      |
      | uuid-docker-1  | docker-debug     | oss       | Help me fix docker compose networking issue  |
      | uuid-k8s-2     | kubernetes-setup | oss       | Setting up kubernetes cluster with helm      |
      | uuid-api-3     | api-design       | acme    | Design RESTful API for user management       |
      | uuid-timeout-4 | error-debug      | oss       | Error: connection timeout after 30 seconds   |

  Scenario: Keyword search finds matching sessions
    When I run "agm session search docker"
    Then the command should succeed
    And the output should contain "1 matching session"
    And the output should show session "uuid-docker-1" with name "docker-debug"
    And the context snippet should contain "docker compose"

  Scenario: Keyword search with multiple matches
    When I run "agm session search kubernetes"
    Then the command should succeed
    And the output should contain "1 matching session"
    And the output should show session "uuid-k8s-2"

  Scenario: Case-insensitive search by default
    When I run "agm session search DOCKER"
    Then the command should succeed
    And the output should contain "1 matching session"
    And the output should show session "uuid-docker-1"

  Scenario: Case-sensitive search
    When I run "agm session search --case-sensitive API"
    Then the command should succeed
    And the output should contain "1 matching session"
    And the match count should be "1"
    # Only matches "API", not "api"

  Scenario: Regex pattern search
    When I run "agm session search --regex 'Error.*timeout'"
    Then the command should succeed
    And the output should contain "1 matching session"
    And the output should show session "uuid-timeout-4"
    And the context snippet should contain "Error: connection timeout"

  Scenario: Workspace filter
    When I run "agm session search --workspace oss kubernetes"
    Then the command should succeed
    And the output should contain "1 matching session"
    And the workspace should be "oss"

  Scenario: Workspace filter excludes other workspaces
    When I run "agm session search --workspace oss API"
    Then the command should succeed
    And the output should contain "No matching sessions found"
    # API content is in acme workspace

  Scenario: No matches found
    When I run "agm session search nonexistent"
    Then the command should succeed
    And the output should contain "No matching sessions found"
    And the output should contain "Tips:"

  Scenario: Multiple matches in same session
    Given session "uuid-docker-1" has conversation:
      | role      | content                                |
      | user      | Help with docker compose networking    |
      | assistant | Let's check your docker-compose.yml    |
      | user      | The docker logs show connection errors |
    When I run "agm session search docker"
    Then the command should succeed
    And the match count for "uuid-docker-1" should be "3"

  Scenario: Invalid regex pattern
    When I run "agm session search --regex '['"
    Then the command should fail
    And the output should contain "invalid regex pattern"

  Scenario: Search with no sessions
    Given I have an empty sessions directory
    When I run "agm session search anything"
    Then the command should succeed
    And the output should contain "No matching sessions found"

  Scenario: Search displays context snippet
    When I run "agm session search timeout"
    Then the command should succeed
    And the output should show a table with columns:
      | UUID       | Session Name | Matches | Workspace | Context                          |
      | uuid-ti... | error-debug  | 1       | oss       | Error: connection timeout after... |

  Scenario: Search counts all occurrences
    Given session "uuid-timeout-4" has 5 occurrences of "timeout"
    When I run "agm session search timeout"
    Then the match count for "uuid-timeout-4" should be "5"

  Scenario: Search ignores orphaned sessions without conversation files
    Given session "uuid-orphan-5" has no conversation file
    When I run "agm session search anything"
    Then the output should not show session "uuid-orphan-5"

  Scenario: Regex with case-insensitive flag
    When I run "agm session search --regex '(?i)error'"
    Then the command should succeed
    And the output should show session "uuid-timeout-4"

  Scenario: Search in specific workspace shows workspace column
    When I run "agm session search --workspace oss kubernetes"
    Then the output table should show workspace "oss"

  Scenario: Empty query string
    When I run "agm session search"
    Then the command should fail
    And the output should contain "requires at least 1 arg"

  Scenario: Search output is formatted as table
    When I run "agm session search kubernetes"
    Then the output should be a table
    And the table should have headers:
      | UUID | Session Name | Matches | Workspace | Context |
