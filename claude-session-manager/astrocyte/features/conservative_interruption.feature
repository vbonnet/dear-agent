Feature: Conservative Interruption Policy
  As a user of the astrocyte daemon
  I want conservative interruption behavior
  So that my legitimate work (AskUserQuestion, planning) is never interrupted

  Background:
    Given astrocyte daemon is running
    And the configuration uses conservative thresholds

  # RULE: Default is "do not interrupt"
  # Only interrupt for genuine freezes (0 tokens bug, UI completely unresponsive)
  # Better to miss an interruption than interrupt legitimate work

  Scenario: Session waiting for AskUserQuestion is NOT interrupted
    Given a session named "planning-session"
    And the session shows AskUserQuestion prompt with options A/B/C
    And the session has completion language "Which approach fits best?"
    And the session has idle prompt "❯"
    And no pending tool calls are visible
    When the cursor remains frozen for 30 minutes
    Then endpoint detection should identify this as "natural completion"
    And no ESC key should be sent
    And no recovery should be attempted
    And the incident log should contain "ENDPOINT DETECTED"
    And the rationale should mention "waiting for user input"

  Scenario: Planning session at completion is NOT interrupted
    Given a session named "architecture-planning"
    And the session shows completion language "✅ Plan finalized"
    And the session shows "Ready for your approval"
    And the session has idle prompt "❯"
    And no pending tool calls are visible
    When the cursor remains frozen for 30 minutes
    Then endpoint detection should identify this as "natural completion"
    And no recovery should be attempted

  Scenario: Session frozen on 0 tokens bug IS interrupted
    Given a session named "zero-tokens-bug"
    And the session shows "Bootstrapping… (esc to interrupt · 15m 32s · ↓ 0 tokens)"
    And no completion language is present
    And no idle prompt is present
    When the 0 tokens pattern persists for 15 minutes
    Then endpoint detection should identify this as "NOT endpoint"
    And ESC key should be sent
    And recovery should be attempted
    And the incident log should contain "STUCK DETECTED"
    And the symptom should be "stuck_zero_token_waiting"

  Scenario: Session stuck mustering for 20+ minutes IS interrupted
    Given a session named "mustering-freeze"
    And the session shows "✻ Mustering..."
    And the mustering pattern persists in consecutive checks
    And no completion language is present
    And no idle prompt is present
    When the mustering pattern persists for 20 minutes
    Then endpoint detection should identify this as "NOT endpoint"
    And ESC key should be sent
    And the symptom should be "stuck_mustering"

  Scenario: Session with UI completely unresponsive IS interrupted
    Given a session named "ui-frozen"
    And the cursor position has not changed
    And no pane output has changed
    And no spinners are visible
    And no completion language is present
    When the cursor remains frozen for 30 minutes
    Then endpoint detection should identify this as "NOT endpoint"
    And ESC key should be sent
    And the symptom should be "stuck_cursor_frozen"

  Scenario: Partial endpoint signals do NOT prevent appropriate detection
    Given a session named "partial-signals"
    And the session has idle prompt "❯"
    And the session shows "Still working on task..."
    And no completion language is present
    When the cursor remains frozen for 30 minutes
    Then endpoint detection should identify this as "NOT endpoint"
    # Has idle prompt but NO completion language - not enough for endpoint
    # Could still be interrupted if genuinely frozen

  Scenario: Multiple detection cycles with consistent endpoint signals
    Given a session named "persistent-endpoint"
    And the session shows "✅ Task completed"
    And the session shows "Ready to proceed"
    And the session has idle prompt "❯"
    When detection cycle 1 runs
    And detection cycle 2 runs 1 minute later
    And detection cycle 3 runs 30 minutes later
    Then all cycles should detect endpoint
    And no recovery should be attempted in any cycle

  Scenario: Completion language with active spinner is NOT an endpoint
    Given a session named "conflicting-signals"
    And the session shows "✅ Task completed"
    And the session also shows "✶ Thinking…"
    And the session has idle prompt "❯"
    When endpoint detection runs
    Then endpoint detection should identify this as "NOT endpoint"
    # Spinner indicates active work - contradicts completion

  Scenario Outline: Various completion language phrases detected
    Given a session shows completion phrase "<completion_phrase>"
    And the session has idle prompt "❯"
    And no pending tool calls are visible
    When endpoint detection runs
    Then the session should be detected as endpoint

    Examples:
      | completion_phrase           |
      | Task completed              |
      | All done                    |
      | Ready to proceed            |
      | Session complete            |
      | ✅ Finished                 |
      | Complete                    |
      | All beads closed            |

  Scenario Outline: Various spinner patterns prevent endpoint detection
    Given a session shows spinner pattern "<spinner_pattern>"
    And the session has idle prompt "❯"
    When endpoint detection runs
    Then the session should NOT be detected as endpoint

    Examples:
      | spinner_pattern |
      | ✶ Thinking…     |
      | ✻ Mustering…    |
      | ✢ Processing…   |
      | Galloping…      |
      | Cogitating…     |
      | Channelling…    |

  Scenario: Conservative thresholds prevent premature interruption
    Given the configuration has "mustering_timeout" set to 20 minutes
    And the configuration has "zero_token_waiting" set to 15 minutes
    And the configuration has "cursor_frozen" set to 30 minutes
    When a session shows mustering for 10 minutes
    Then no recovery should be attempted
    # Mustering threshold is 20 min - 10 min is below threshold

  Scenario: Better to miss than interrupt legitimate work
    Given a session that may be waiting for user input
    And endpoint signals are ambiguous (50% confidence)
    When detection runs
    Then the default behavior should be "do not interrupt"
    And the session should be treated as endpoint
    # Conservative policy: when uncertain, do NOT interrupt
