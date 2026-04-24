Feature: Session Creation with --mode Flag
  As an AGM user
  I want to specify a permission mode when creating a new session
  So that the session starts in the desired mode without manual switching

  Background:
    Given I have AGM installed
    And I have Claude CLI installed

  Scenario: Create session with --mode=plan
    Given no session named "test-mode-plan" exists
    When I run "agm session new test-mode-plan --harness claude-code --mode plan"
    Then the command should succeed within 90 seconds
    And a tmux session named "test-mode-plan" should exist
    And the session manifest should show permission_mode "plan"

  Scenario: Create session with --mode=auto
    Given no session named "test-mode-auto" exists
    When I run "agm session new test-mode-auto --harness claude-code --mode auto"
    Then the command should succeed within 90 seconds
    And the session manifest should show permission_mode "auto"

  Scenario: Create session with --mode=default is a no-op
    Given no session named "test-mode-default" exists
    When I run "agm session new test-mode-default --harness claude-code --mode default"
    Then the command should succeed within 90 seconds
    And the session should start in default mode

  Scenario: Create session without --mode stays in default mode
    Given no session named "test-no-mode" exists
    When I run "agm session new test-no-mode --harness claude-code"
    Then the command should succeed within 90 seconds
    And the session should start in default mode

  Scenario: Invalid --mode value is rejected
    When I run "agm session new test-bad-mode --harness claude-code --mode turbo"
    Then the command should fail
    And I should see an error message containing "invalid --mode"

  Scenario: --mode with --prompt sends mode before prompt
    Given no session named "test-mode-prompt" exists
    When I run "agm session new test-mode-prompt --harness claude-code --mode plan --prompt 'hello'"
    Then the command should succeed within 90 seconds
    And the mode should be switched before the prompt is sent

  Scenario: --mode with codex-cli warns but continues
    Given no session named "test-mode-codex" exists
    When I run "agm session new test-mode-codex --harness codex-cli --mode plan"
    Then I should see a warning about mode switching
    And the session should still be created
