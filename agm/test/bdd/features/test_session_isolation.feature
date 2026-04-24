Feature: Test Session Isolation and Cleanup
  As an AGM user
  I want test sessions to be properly isolated from production sessions
  So that my production workspace remains clean and organized

  Background:
    Given AGM is installed and configured
    And the PreToolUse hook is installed

  # Layer 1: Cleanup (Reactive)
  Scenario: Clean up orphaned test sessions with interactive selection
    Given I have test sessions in production workspace:
      | session_name     | message_count |
      | test-experiment  | 2             |
      | test-temp        | 1             |
      | test-important   | 15            |
    When I run "agm admin cleanup-test-sessions"
    Then I should see an interactive selection prompt
    And sessions with less than 5 messages should be pre-selected
    And session "test-important" should not be pre-selected
    When I confirm the selection
    Then backups should be created in "~/.agm/backups/sessions/"
    And selected test sessions should be removed from production workspace
    And session "test-important" should remain in production workspace

  Scenario: Dry-run cleanup shows preview without deletion
    Given I have test sessions in production workspace:
      | session_name    | message_count |
      | test-foo        | 3             |
      | test-bar        | 2             |
    When I run "agm admin cleanup-test-sessions --dry-run"
    Then I should see a list of sessions that would be cleaned
    And I should see message counts for each session
    And no sessions should be deleted
    And no backups should be created

  Scenario: Cleanup creates backups before deletion
    Given I have a test session "test-backup-check" with 3 messages
    When I run "agm admin cleanup-test-sessions --auto-yes"
    Then a backup should be created at "~/.agm/backups/sessions/test-backup-check-{timestamp}.tar.gz"
    And the backup should contain the full session directory
    And the backup should preserve file permissions
    And the session should be deleted from production workspace

  # Layer 2: Interactive Prevention (Educational)
  Scenario: Creating test-pattern session shows interactive prompt
    When I run "agm session new test-experiment"
    Then I should see an interactive prompt with options:
      | option                                | description                          |
      | Use --test flag (required)            | Creates isolated test session        |
      | Cancel and rename to non-test name   | Cancel creation and choose new name  |
    And the prompt should explain why test sessions matter
    And the prompt should show expected behavior for each option

  Scenario: Choosing --test flag option creates isolated session
    When I run "agm session new test-experiment"
    And I select "Use --test flag (required)" from the prompt
    Then a session should be created in "~/sessions-test/"
    And the tmux session should be prefixed with "agm-test-"
    And the session should not be tracked in the AGM database
    And I should see confirmation: "✓ Using --test flag for isolated test session"

  Scenario: Choosing cancel option aborts session creation
    When I run "agm session new test-experiment"
    And I select "Cancel and rename to non-test name" from the prompt
    Then no session should be created
    And I should see: "❌ Cancelled"
    And I should see suggested alternatives

  Scenario: Using --test flag directly skips prompt
    When I run "agm session new --test test-experiment"
    Then no interactive prompt should be shown
    And a session should be created in "~/sessions-test/"
    And the tmux session should be prefixed with "agm-test-"

  Scenario: Using --allow-test-name flag bypasses prompt
    When I run "agm session new test-auth-flow --allow-test-name"
    Then no interactive prompt should be shown
    And a session should be created in production workspace
    And I should see warning about production session

  # Layer 3: Automated Prevention (Hook)
  Scenario: PreToolUse hook blocks test-pattern without --test flag
    When Claude Code attempts to run "agm session new test-foo"
    Then the PreToolUse hook should block the command
    And I should see error message: "❌ Test Session Pattern Detected"
    And the error should suggest using "--test" flag
    And the error should show the correct command: "agm session new --test test-foo"
    And the error should mention the "--allow-test-name" override option

  Scenario: Hook allows test-pattern with --test flag
    When Claude Code attempts to run "agm session new --test test-foo"
    Then the PreToolUse hook should allow the command
    And no error message should be displayed
    And the session should be created in "~/sessions-test/"

  Scenario: Hook allows test-pattern with --allow-test-name flag
    When Claude Code attempts to run "agm session new test-foo --allow-test-name"
    Then the PreToolUse hook should allow the command
    And no error message should be displayed
    And the session should be created in production workspace

  Scenario: Hook allows legitimate names with 'test' substring
    When Claude Code attempts to run "agm session new my-testing-feature"
    Then the PreToolUse hook should allow the command
    And no error message should be displayed

  # Edge Cases
  Scenario Outline: Test pattern detection handles various case formats
    When Claude Code attempts to run "agm session new <session_name>"
    Then the PreToolUse hook should <action> the command
    And I should <error_expectation>

    Examples:
      | session_name     | action | error_expectation              |
      | test-foo         | block  | see error message              |
      | TEST-FOO         | block  | see error message              |
      | Test-Foo         | block  | see error message              |
      | test-            | block  | see error message              |
      | test             | allow  | not see error message          |
      | my-test-feature  | allow  | not see error message          |
      | latest           | allow  | not see error message          |
      | contest          | allow  | not see error message          |

  Scenario: Hook gracefully degrades on errors
    Given the PreToolUse hook encounters an internal error
    When Claude Code attempts to run "agm session new test-foo"
    Then the hook should allow the command (graceful degradation)
    And an error should be logged for debugging
    But the command should not be blocked

  # Integration Tests
  Scenario: End-to-end test session workflow
    When I run "agm session new --test integration-test"
    Then a session should be created in "~/sessions-test/integration-test/"
    And the tmux session should be named "agm-test-integration-test"
    When I work in the session and exit
    And I later clean up test sessions
    Then "integration-test" should be easily removed
    And no trace should remain in production workspace

  Scenario: Cleanup respects message threshold
    Given I have test sessions with various message counts:
      | session_name | message_count |
      | test-a       | 2             |
      | test-b       | 5             |
      | test-c       | 10            |
    When I run "agm admin cleanup-test-sessions --message-threshold 5"
    Then only "test-a" should be selected for cleanup
    And "test-b" and "test-c" should be preserved
    And I should see the threshold in the selection UI

  Scenario: Cleanup supports custom patterns
    Given I have sessions:
      | session_name       |
      | test-foo           |
      | experiment-bar     |
      | temp-session       |
    When I run "agm admin cleanup-test-sessions --pattern '^(test|temp)-'"
    Then "test-foo" and "temp-session" should be selected
    And "experiment-bar" should not be selected

  # Documentation Verification
  Scenario: User can find test session documentation
    When I run "agm session new --help"
    Then I should see documentation for "--test" flag
    And I should see documentation for "--allow-test-name" flag
    When I check the docs at "docs/TEST-SESSION-GUIDE.md"
    Then I should find comprehensive examples
    And I should find a comparison table
    And I should find troubleshooting guidance

  # Metrics and Monitoring
  Scenario: Cleanup provides detailed reporting
    Given I have 5 test sessions to clean up
    When I run "agm admin cleanup-test-sessions --auto-yes"
    Then I should see a summary report with:
      | metric                | value |
      | Sessions scanned      | 5     |
      | Sessions backed up    | 5     |
      | Sessions deleted      | 5     |
      | Backups created       | 5     |
    And each backup path should be displayed
    And total cleanup time should be shown

  # Safety and Rollback
  Scenario: Backup can be manually restored
    Given I have cleaned up session "test-important" by mistake
    And a backup exists at "~/.agm/backups/sessions/test-important-{timestamp}.tar.gz"
    When I manually extract the backup to "~/.claude/sessions/"
    Then the session should be fully restored
    And all conversation history should be intact
    And the manifest should be valid
