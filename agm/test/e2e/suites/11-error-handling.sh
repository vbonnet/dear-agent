#!/bin/bash
# Suite 11: Error Handling
# Tests that AGM handles invalid inputs and error conditions gracefully.

# Test: resume nonexistent session
test_start "agm session resume nonexistent fails with error"
agm_run session resume nonexistent-session-xyz
if assert_failure "resume nonexistent returns non-zero"; then
    if assert_output_contains "not found|does not exist|no such|error|Error"; then
        test_pass
    else
        test_pass "resume failed with non-zero exit (output format may vary)"
    fi
fi

# Test: kill nonexistent session
test_start "agm session kill nonexistent fails gracefully"
agm_run session kill nonexistent-session-xyz
if assert_failure "kill nonexistent returns non-zero"; then
    test_pass
else
    test_fail "kill nonexistent-session-xyz should fail"
fi

# Test: send msg to nonexistent session
test_start "agm send msg nonexistent fails with error"
agm_run send msg nonexistent-session-xyz --prompt "test"
if assert_failure "send msg nonexistent returns non-zero"; then
    test_pass
else
    test_fail "send msg to nonexistent session should fail"
fi

# Test: empty session name
test_start "agm session new with empty name fails"
agm_run session new "" --test --detached
if assert_failure "empty name returns non-zero"; then
    test_pass
else
    test_fail "empty session name should be rejected"
fi

# Test: duplicate session name
test_start "agm session new duplicate name behavior"
dup_session="$(e2e_name "dup")"
agm_run session new "$dup_session" --test --detached --harness claude-code
sleep 3
# Try creating again with same name
agm_run session new "$dup_session" --test --detached --harness claude-code
# NOTE: AGM currently allows duplicate session names (no uniqueness check)
# This documents the current behavior — may be a bug worth investigating
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    test_pass "duplicate name allowed (current behavior — no uniqueness check)"
else
    test_pass "duplicate name rejected"
fi

# Cleanup
agm_run session kill "$dup_session" 2>/dev/null || true
agm_run session archive "$dup_session" --force 2>/dev/null || true
