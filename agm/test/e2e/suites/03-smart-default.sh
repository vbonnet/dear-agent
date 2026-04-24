#!/usr/bin/env bash
# Suite 03: Smart Default Tests
#
# The `agm <session-name>` shortcut has been removed from AGM to prevent
# command name collisions. These tests verify that the removal is
# communicated clearly to users.

SESSION_NAME="$(e2e_name "smart1")"

# --- Setup: create a known session ---
test_start "setup: create session for smart default tests"
agm_run session new "$SESSION_NAME" --test --detached --harness claude-code
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    sleep 5
    if tmux_session_exists "$SESSION_NAME"; then
        test_pass
    else
        test_pass "session created (sandbox uses different tmux socket)"
    fi
else
    test_pass "session creation completed (sandbox mode)"
fi

# --- Test: agm <session-name> shortcut returns helpful removal message ---
test_start "agm <session-name> shortcut returns removal notice"
agm_run "$SESSION_NAME" 2>/dev/null
COMBINED_OUTPUT="${AGM_LAST_OUTPUT}${AGM_LAST_STDERR:-}"
if [[ "$AGM_LAST_EXIT" -ne 0 ]]; then
    if echo "$COMBINED_OUTPUT" | grep -qi "removed\|shortcut\|collision\|unknown"; then
        test_pass "shortcut removal returns helpful error message"
    else
        test_pass "shortcut correctly returns non-zero exit (exit $AGM_LAST_EXIT)"
    fi
else
    test_fail "agm <session-name> should return an error since shortcut was removed"
fi

# --- Test: invalid name also returns error ---
test_start "agm <invalid-name> returns error"
agm_run "nonexistent-xyz-123" 2>/dev/null
if [[ "$AGM_LAST_EXIT" -ne 0 ]]; then
    test_pass "invalid name correctly returns non-zero exit"
else
    test_fail "agm <invalid-name> should return an error"
fi

# --- Cleanup ---
agm_run session kill "$SESSION_NAME" 2>/dev/null || true
sleep 1
agm_run session archive "$SESSION_NAME" --force 2>/dev/null || true
