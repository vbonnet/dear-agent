Feature: Bow Gate 9 - Session Registry Integrity
  As a bow user
  I want session integrity verified before completing a bow session
  So that I don't lose work due to orphaned conversations or manifest issues

  Background:
    Given I have bow-core installed with hook system
    And I have AGM installed
    And Gate 9 is registered in ~/.engram/hooks.toml

  Scenario: Gate 9 passes with healthy session
    Given I am in an AGM session "healthy-session"
    And the session has a valid manifest
    And the manifest UUID matches the active conversation
    And the tmux session exists and matches the manifest
    And no orphaned sessions exist in the workspace
    When I run "/bow"
    Then Gate 9 should execute
    And Gate 9 should pass
    And the bow output should show "✓ Gate 9: Session Registry Integrity - PASSED"

  Scenario: Gate 9 fails when current session has no manifest
    Given I am in a Claude conversation
    But the conversation has no AGM manifest
    When I run "/bow"
    Then Gate 9 should execute
    And Gate 9 should fail with severity "HIGH"
    And the violation should be "Current session is orphaned (no AGM manifest)"
    And the remediation should suggest "Run: agm session import <uuid>"
    And bow should not complete until remediation

  Scenario: Gate 9 warns about workspace orphans
    Given I am in an AGM session "current-session"
    And the session has a valid manifest
    But 2 orphaned sessions exist in the same workspace
    When I run "/bow"
    Then Gate 9 should execute
    And Gate 9 should warn with severity "MEDIUM"
    And the violation should be "2 orphaned conversations in workspace 'oss'"
    And the remediation should suggest "Run: agm admin find-orphans --workspace oss --auto-import"
    And bow should allow continuing but show warning

  Scenario: Gate 9 detects manifest-tmux mismatch
    Given I am in an AGM session "test-session"
    And the manifest has tmux_session_name "agm-oss-test-session"
    But the actual tmux session is named "agm-oss-different-name"
    When I run "/bow"
    Then Gate 9 should fail with severity "HIGH"
    And the violation should be "Tmux session name mismatch"
    And the output should show expected vs actual names

  Scenario: Gate 9 hook registration
    Given bow-core hook system is initialized
    When I check ~/.engram/hooks.toml
    Then there should be a hook registered:
      """
      [[hooks]]
      name = "session-registry-integrity"
      event = "session-completion"
      priority = 4
      type = "binary"
      command = "agm"
      args = ["admin", "audit", "--current-session", "--json"]
      timeout = 60
      """

  Scenario: Gate 9 returns VerificationResult JSON
    Given I am in an AGM session with an issue
    When Gate 9 executes
    Then the output should be valid VerificationResult JSON
    And the JSON should have fields:
      | Field       | Value                          |
      | gate_name   | Session Registry Integrity     |
      | passed      | false                          |
      | severity    | HIGH                           |
      | violations  | [array of violation objects]   |
    And each violation should have "type", "description", "remediation"

  Scenario: Gate 9 checks all integrity issues
    Given I am in an AGM session
    When Gate 9 executes via "agm admin audit --current-session --json"
    Then it should check:
      | Check                                      |
      | Current session has AGM manifest           |
      | Manifest UUID matches conversation UUID    |
      | Tmux session exists                        |
      | Tmux session name matches manifest         |
      | No orphaned conversations in workspace     |
      | Manifest is not corrupted                  |

  Scenario: Gate 9 hook execution timeout
    Given Gate 9 is registered with 60 second timeout
    And "agm admin audit --current-session" hangs
    When bow executes Gate 9
    Then the hook should timeout after 60 seconds
    And bow should report "Gate 9 timed out"
    And bow should fail the session completion

  Scenario: Gate 9 hook execution failure
    Given Gate 9 is registered in hooks.toml
    But the agm binary is not in PATH
    When bow executes Gate 9
    Then bow should report "Gate 9 hook failed to execute"
    And the error should show "agm command not found"
    And bow should fail the session completion

  Scenario: Gate 9 allows clean workspace
    Given I am in an AGM session
    And all sessions in the workspace are healthy
    And no orphaned conversations exist
    And all manifests are valid
    When I run "/bow"
    Then Gate 9 should pass
    And the VerificationResult should have passed=true
    And the violations array should be empty

  Scenario: Gate 9 remediation workflow
    Given I am in an orphaned conversation
    When I run "/bow"
    Then Gate 9 should fail
    And the output should suggest "agm session import <uuid>"
    When I run "agm session import <uuid>"
    And I run "/bow" again
    Then Gate 9 should pass
    And bow should complete successfully

  Scenario: Gate 9 priority in hook chain
    Given bow-core has multiple gates registered
    And Gate 9 has priority 4
    When bow executes session-completion hooks
    Then Gate 9 should execute after conversation/questions gates (priority 1-3)
    And Gate 9 should execute before cleanup gates (priority 5+)

  Scenario: Gate 9 workspace-aware checking
    Given I have sessions in workspaces "oss", "acme", "research"
    And I am in a session in workspace "oss"
    And workspace "acme" has orphaned sessions
    When Gate 9 executes
    Then it should only check orphans in workspace "oss"
    And it should not report orphans from "acme" workspace

  Scenario: Gate 9 skips archived sessions
    Given I am in an AGM session
    And an archived session exists in the workspace
    When Gate 9 executes
    Then the archived session should be excluded from checks
    And the archived session should not trigger orphan warnings
