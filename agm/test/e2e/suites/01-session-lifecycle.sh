#!/usr/bin/env bash
# Suite 01: Session Lifecycle Tests
# Tests: new, list, kill, resume, archive, unarchive
#
# NOTE: This suite does NOT use --test flag because lifecycle commands
# (list, kill, resume, archive, unarchive) require Dolt integration.
# Uses unique timestamped names + aggressive cleanup to keep production clean.

S1=$(e2e_name "lc1")
S2=$(e2e_name "lc2")

# --- Test: session list (baseline) ---
test_start "agm session list (baseline)"
agm_run session list
if assert_exit_code 0 "session list exits 0"; then
    test_pass
fi

# --- Test: session new --detached ---
test_start "agm session new --detached"
agm_run session new "$S1" --detached --harness claude-code
if assert_exit_code 0 "session new exits 0"; then
    sleep 8
    if agm_session_exists "$S1"; then
        test_pass
    else
        test_fail "$S1 not found in session list"
    fi
fi

# --- Test: session new with --prompt ---
test_start "agm session new with --prompt"
agm_run session new "$S2" --detached --harness claude-code --prompt "say hello"
if assert_exit_code 0 "session new with prompt exits 0"; then
    sleep 8
    if agm_session_exists "$S2"; then
        test_pass
    else
        test_fail "$S2 not found in session list"
    fi
fi

# --- Test: session list shows new sessions ---
test_start "agm session list shows created sessions"
agm_run session list
if assert_exit_code 0; then
    if assert_output_contains "$S1"; then
        test_pass
    fi
fi

# --- Test: session list --json ---
test_start "agm session list --json"
agm_run session list --json
if assert_exit_code 0 "list --json exits 0"; then
    test_pass
fi

# --- Test: session kill ---
test_start "agm session kill"
agm_run session kill "$S1"
if assert_exit_code 0 "session kill exits 0"; then
    sleep 3
    test_pass
fi

# --- Test: session list after kill ---
test_start "agm session list after kill"
agm_run session list
if assert_exit_code 0; then
    test_pass
fi

# --- Test: session resume from stopped ---
test_start "agm session resume from stopped"
agm_run session resume "$S1" --detached
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    sleep 8
    if agm_session_exists "$S1"; then
        test_pass
    else
        test_fail "session not visible after resume"
    fi
else
    test_fail "resume command failed" "$AGM_LAST_OUTPUT"
fi

# --- Test: session archive ---
test_start "agm session archive"
agm_run session kill "$S1"
sleep 5
agm_run session archive "$S1"
if [[ "$AGM_LAST_EXIT" -eq 0 ]]; then
    agm_run session list
    if ! echo "$AGM_LAST_OUTPUT" | grep -q "$S1"; then
        test_pass
    else
        test_fail "archived session still in default list"
    fi
else
    agm_run session archive --async "$S1"
    if assert_exit_code 0 "archive --async exits 0"; then
        sleep 5
        test_pass "archived via --async"
    fi
fi

# --- Test: session list --all shows archived session ---
test_start "agm session list --all shows archived"
agm_run session list --all
if assert_exit_code 0; then
    if assert_output_contains "$S1"; then
        test_pass
    fi
fi

# --- Test: session unarchive ---
test_start "agm session unarchive"
agm_run session unarchive "$S1" --force
if assert_exit_code 0 "unarchive exits 0"; then
    test_pass
fi

# --- Cleanup (aggressive) ---
agm_run session kill "$S1" 2>/dev/null || true
sleep 2
agm_run session archive "$S1" 2>/dev/null || true
agm_run session archive --async "$S1" 2>/dev/null || true
agm_run session kill "$S2" 2>/dev/null || true
sleep 2
agm_run session archive "$S2" 2>/dev/null || true
agm_run session archive --async "$S2" 2>/dev/null || true
