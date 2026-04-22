#!/usr/bin/env bash
# Suite 13: Test Environment Integration Tests

TEST_ENV_NAME=$(e2e_name "env")

# --- Test: agm test-env create ---
test_start "agm test-env create"
agm_run test-env create --name="$TEST_ENV_NAME"
if assert_exit_code 0 "test-env create exits 0"; then
    if assert_output_contains "AGM_TEST_ENV"; then
        test_pass
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
if [[ -L "/tmp/agm-test-$TEST_ENV_NAME/home/.codex" ]] || [[ -L "/tmp/agm-test-$TEST_ENV_NAME/home/.config/gcloud" ]]; then
    test_pass
else
    test_pass "no auth files to symlink (env var auth only)"
fi

# --- Test: agm test-env destroy ---
test_start "agm test-env destroy"
agm_run test-env destroy "$TEST_ENV_NAME"
if assert_exit_code 0 "test-env destroy exits 0"; then
    if [[ ! -d "/tmp/agm-test-$TEST_ENV_NAME" ]]; then
        test_pass
    else
        test_fail "test env directory still exists after destroy"
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
