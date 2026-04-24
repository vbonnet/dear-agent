Feature: UX Consistency
  As a user of AGM
  I want consistent UX across all commands
  So that I can learn the CLI quickly and work efficiently

  Background:
    Given I have AGM installed

  Scenario Outline: All commands support --help flag
    When I run "agm <command> --help"
    Then the command should exit with code 0
    And the output should contain "Usage:"
    And the output should contain "Flags:"

    Examples:
      | command |
      | new     |
      | list    |
      | attach  |
      | search  |
      | archive |

  Scenario: Success messages use checkmark prefix
    When I create a new session "test-success"
    Then the output should contain "✅"
    And the output should match pattern "Session.*created"

  Scenario: Error messages follow template format
    When I try to access a non-existent session "missing-session"
    Then the command should exit with non-zero code
    And the output should contain one of:
      | Error: |
      | ❌     |

  Scenario Outline: Commands support standard flags
    When I run "agm <command> --help"
    Then the output should mention "--help"
    And the output should mention flag "<flag>"

    Examples:
      | command | flag    |
      | list    | --format |
      | new     | --debug  |
      | search  | --filter |

  Scenario: Table output follows consistent format
    Given I have multiple sessions
    When I run "agm list --format table"
    Then the output should have aligned columns
    And the first line should be a header row

  Scenario: Interactive commands support --yes flag
    Given I have a session "to-delete"
    When I run "agm archive to-delete --yes"
    Then the command should complete without prompts
    And the command should exit with code 0
