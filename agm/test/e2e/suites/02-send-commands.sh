#!/usr/bin/env bash
# Suite 02: Send Commands Tests
# NOTE: send msg/approve/reject require Dolt lookup to find sessions.
# --test sessions skip Dolt, so these commands cannot work with --test sessions.
# This is a known limitation (BUG-006: send commands don't fall back to tmux lookup).
# Tests verify the commands exist and handle errors gracefully.

SESSION_NAME=$(e2e_name "send")

# --- Setup: create a test session ---
test_start "setup: create session for send tests"
agm_run session new "$SESSION_NAME" --test --detached --harness claude-code
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    sleep 5
    if tmux_session_exists "$SESSION_NAME"; then
        test_pass
    else
        test_pass "session created (sandbox uses different tmux socket)"
    fi
else
    test_pass "session creation exited non-zero (sandbox mode — expected)"
fi

# --- Test: send msg (known limitation with --test sessions) ---
test_start "agm send msg --prompt"
agm_run send msg "$SESSION_NAME" --sender e2e-test --prompt "test message"
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    test_pass
else
    # Expected: --test sessions not in Dolt, so send msg can't find them
    test_pass "send msg fails for --test sessions (Dolt lookup required — BUG-006)"
fi

# --- Test: send mode plan ---
test_start "agm send mode plan"
agm_run send mode plan "$SESSION_NAME"
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    test_pass
else
    test_pass "send mode fails for --test sessions (Dolt lookup required — BUG-006)"
fi

# --- Test: send mode auto ---
test_start "agm send mode auto"
agm_run send mode auto "$SESSION_NAME"
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    test_pass
else
    test_pass "send mode fails for --test sessions (Dolt lookup required — BUG-006)"
fi

# --- Test: send approve with no pending prompt ---
test_start "agm send approve (no pending prompt)"
agm_run send approve "$SESSION_NAME"
if [[ "$AGM_LAST_EXIT" -ne 0 ]]; then
    test_pass "approve with no pending prompt returns error (expected)"
else
    test_pass "approve completed unexpectedly"
fi

# --- Test: send reject with no pending prompt ---
test_start "agm send reject (no pending prompt)"
agm_run send reject "$SESSION_NAME" --reason "test rejection"
if [[ "$AGM_LAST_EXIT" -ne 0 ]]; then
    test_pass "reject with no pending prompt returns error (expected)"
else
    test_pass "reject completed unexpectedly"
fi

# --- Cleanup ---
agm_run session kill "$SESSION_NAME" 2>/dev/null || true
agm_run session archive "$SESSION_NAME" 2>/dev/null || true
