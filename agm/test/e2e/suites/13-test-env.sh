#!/usr/bin/env bash
# Suite 13: Test Environment Integration Tests

TEST_ENV_NAME=$(e2e_name "env")
TEST_ENV_BASE=""

# --- Test: agm test-env create ---
test_start "agm test-env create"
agm_run test-env create --name="$TEST_ENV_NAME"
if assert_exit_code 0 "test-env create exits 0"; then
    if assert_output_contains "AGM_TEST_ENV"; then
        TEST_ENV_BASE=$(printf '%s\n' "$AGM_LAST_OUTPUT" | sed -n 's/^Base: //p' | tail -1)
        if [[ -d "$TEST_ENV_BASE" && "$TEST_ENV_BASE" == /tmp/agm-u-*/agm-test-"$TEST_ENV_NAME" ]]; then
            test_pass
        else
            test_fail "test-env create reports its owned base" "$TEST_ENV_BASE"
        fi
    fi
fi

# --- Test: agm test-env list ---
test_start "agm test-env list shows created env"
agm_run test-env list
if assert_exit_code 0; then
    if assert_output_contains "$TEST_ENV_NAME"; then
        test_pass
    fi
fi

# --- Test: auth symlinks exist ---
test_start "auth symlinks created"
if [[ ! -d "$TEST_ENV_BASE/home" ]]; then
    test_fail "test environment home is missing" "$TEST_ENV_BASE/home"
elif [[ -e "$HOME/.codex" && ! -L "$TEST_ENV_BASE/home/.codex" ]]; then
    test_fail "existing Codex credentials were not linked"
elif [[ -e "$HOME/.config/gcloud" && ! -L "$TEST_ENV_BASE/home/.config/gcloud" ]]; then
    test_fail "existing gcloud credentials were not linked"
else
    test_pass
fi

# --- Test: agm test-env destroy ---
test_start "agm test-env destroy"
agm_run test-env destroy "$TEST_ENV_NAME"
if assert_exit_code 0 "test-env destroy exits 0"; then
    if [[ -n "$TEST_ENV_BASE" && ! -e "$TEST_ENV_BASE" && ! -e "$TEST_ENV_BASE.sock" ]]; then
        test_pass
    else
        test_fail "test env state still exists after destroy" "$TEST_ENV_BASE"
    fi
fi

# --- Test: agm test-env list (empty after destroy) ---
test_start "agm test-env list (empty after destroy)"
agm_run test-env list
if assert_exit_code 0; then
    if ! echo "$AGM_LAST_OUTPUT" | grep -q "$TEST_ENV_NAME"; then
        test_pass
    else
        test_fail "destroyed env still in list"
    fi
fi
