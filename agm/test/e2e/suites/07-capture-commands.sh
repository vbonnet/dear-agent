#!/bin/bash
# Suite 07: Capture Commands
# Tests agm capture with various output formats and options.
# Requires Dolt adapter connection.

SESSION_NAME="$(e2e_name "cap")"

# Setup: create a test session and send some content
agm_run session new "$SESSION_NAME" --test --detached --harness claude-code
sleep 5
agm_run send msg "$SESSION_NAME" --sender e2e-test --prompt "hello capture test content"
sleep 3

# Test agm capture (basic)
test_start "agm capture returns pane content"
if ! dolt_check_available; then
    test_skip "Dolt adapter required for capture"
else
    agm_run capture "$SESSION_NAME"
    if assert_exit_code 0; then
        if assert_output_not_empty; then
            test_pass
        else
            test_fail "capture returned empty output"
        fi
    fi
fi

# Test agm capture --json
test_start "agm capture --json returns valid JSON with metadata"
if ! dolt_check_available; then
    test_skip "Dolt adapter required for capture"
else
    agm_run capture "$SESSION_NAME" --json
    if assert_exit_code 0; then
        if assert_output_contains "session|pane|content|timestamp"; then
            test_pass
        else
            test_fail "JSON output missing expected metadata fields"
        fi
    fi
fi

# Test agm capture --lines
test_start "agm capture --lines 5 limits output"
if ! dolt_check_available; then
    test_skip "Dolt adapter required for capture"
else
    agm_run capture "$SESSION_NAME" --lines 5
    if assert_exit_code 0; then
        line_count=$(echo "$AGM_LAST_OUTPUT" | wc -l)
        if [[ "$line_count" -le 6 ]]; then
            test_pass
        else
            test_fail "expected at most 5 lines, got $line_count"
        fi
    fi
fi

# Test agm capture --tail
test_start "agm capture --tail 3 returns tail output"
if ! dolt_check_available; then
    test_skip "Dolt adapter required for capture"
else
    agm_run capture "$SESSION_NAME" --tail 3
    if assert_exit_code 0; then
        line_count=$(echo "$AGM_LAST_OUTPUT" | wc -l)
        if [[ "$line_count" -le 4 ]]; then
            test_pass
        else
            test_fail "expected at most 3 tail lines, got $line_count"
        fi
    fi
fi

# Cleanup
agm_run session kill "$SESSION_NAME" 2>/dev/null || true
agm_run session archive "$SESSION_NAME" --force 2>/dev/null || true
