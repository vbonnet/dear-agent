#!/usr/bin/env bash
# AGM E2E Test Helpers
# TAP (Test Anything Protocol) output format

set -euo pipefail

# --- Configuration ---
export AGM_E2E_SOCKET="${AGM_E2E_SOCKET:-/tmp/agm-e2e.sock}"
export AGM_E2E_RESULTS_DIR="${AGM_E2E_RESULTS_DIR:-/tmp/agm-e2e-results}"
export AGM_E2E_TIMEOUT="${AGM_E2E_TIMEOUT:-30}"

# Generate unique test prefix using timestamp to avoid name collisions
E2E_RUN_ID="${E2E_RUN_ID:-$(date -u +%Y%m%dT%H%M%S)}"
E2E_PREFIX="e2e-${E2E_RUN_ID}"

# Helper to generate unique session names per test run
e2e_name() {
    local suffix="$1"
    echo "${E2E_PREFIX}-${suffix}"
}

# --- State ---
_TEST_COUNT=0
_TEST_PASS=0
_TEST_FAIL=0
_TEST_SKIP=0
_CURRENT_TEST=""

# --- TAP Output ---
test_start() {
    local name="$1"
    _CURRENT_TEST="$name"
    _TEST_COUNT=$((_TEST_COUNT + 1))
    printf "# Test %d: %s\n" "$_TEST_COUNT" "$name"
}

test_pass() {
    local msg="${1:-$_CURRENT_TEST}"
    _TEST_PASS=$((_TEST_PASS + 1))
    printf "ok %d - %s\n" "$_TEST_COUNT" "$msg"
}

test_fail() {
    local msg="${1:-$_CURRENT_TEST}"
    local detail="${2:-}"
    _TEST_FAIL=$((_TEST_FAIL + 1))
    printf "not ok %d - %s\n" "$_TEST_COUNT" "$msg"
    if [[ -n "$detail" ]]; then
        printf "  ---\n  message: %s\n  ...\n" "$detail"
    fi
}

test_skip() {
    local msg="${1:-$_CURRENT_TEST}"
    local reason="${2:-}"
    _TEST_SKIP=$((_TEST_SKIP + 1))
    _TEST_COUNT=$((_TEST_COUNT + 1))
    printf "ok %d - %s # SKIP %s\n" "$_TEST_COUNT" "$msg" "$reason"
}

test_summary() {
    printf "\n# === Test Summary ===\n"
    printf "# Total:   %d\n" "$_TEST_COUNT"
    printf "# Passed:  %d\n" "$_TEST_PASS"
    printf "# Failed:  %d\n" "$_TEST_FAIL"
    printf "# Skipped: %d\n" "$_TEST_SKIP"
    printf "1..%d\n" "$_TEST_COUNT"
    return "$_TEST_FAIL"
}

# --- AGM Execution ---
agm_run() {
    # Run agm command, capture stdout+stderr and exit code
    local output
    local exit_code=0
    output=$(agm "$@" 2>&1) || exit_code=$?
    AGM_LAST_OUTPUT="$output"
    AGM_LAST_EXIT="$exit_code"
    return 0  # always return 0 so caller can check
}

# --- Assertions ---
assert_exit_code() {
    local expected="$1"
    local label="${2:-command exit code}"
    if [[ "$AGM_LAST_EXIT" -eq "$expected" ]]; then
        return 0
    else
        test_fail "$label" "expected exit code $expected, got $AGM_LAST_EXIT"
        printf "  # Output: %s\n" "$AGM_LAST_OUTPUT" | head -5
        return 1
    fi
}

assert_output_contains() {
    local pattern="$1"
    local label="${2:-output contains '$pattern'}"
    if echo "$AGM_LAST_OUTPUT" | grep -qE "$pattern"; then
        return 0
    else
        test_fail "$label" "output does not match pattern: $pattern"
        printf "  # Output: %s\n" "$AGM_LAST_OUTPUT" | head -5
        return 1
    fi
}

assert_output_not_contains() {
    local pattern="$1"
    local label="${2:-output does not contain '$pattern'}"
    if ! echo "$AGM_LAST_OUTPUT" | grep -qE "$pattern"; then
        return 0
    else
        test_fail "$label" "output unexpectedly matches pattern: $pattern"
        return 1
    fi
}

assert_success() {
    assert_exit_code 0 "${1:-command succeeds}"
}

assert_failure() {
    if [[ "$AGM_LAST_EXIT" -ne 0 ]]; then
        return 0
    else
        test_fail "${1:-command fails}" "expected non-zero exit code, got 0"
        return 1
    fi
}

# --- Tmux Helpers ---
# AGM uses /tmp/agm.sock by default for its tmux sessions
AGM_TMUX_SOCKET="${AGM_TMUX_SOCKET:-/tmp/agm.sock}"

tmux_cmd() {
    tmux -S "$AGM_TMUX_SOCKET" "$@"
}

tmux_capture() {
    local session="$1"
    local lines="${2:-50}"
    tmux_cmd capture-pane -t "$session" -p -S "-$lines" 2>/dev/null || echo ""
}

tmux_wait_for() {
    # Poll tmux pane until pattern appears or timeout
    local session="$1"
    local pattern="$2"
    local timeout="${3:-$AGM_E2E_TIMEOUT}"
    local interval="${4:-1}"
    local elapsed=0

    while [[ "$elapsed" -lt "$timeout" ]]; do
        local content
        content=$(tmux_capture "$session" 100)
        if echo "$content" | grep -qE "$pattern"; then
            return 0
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done
    return 1  # timeout
}

tmux_session_exists() {
    local session="$1"
    tmux_cmd has-session -t "$session" 2>/dev/null
}

tmux_send_keys() {
    local session="$1"
    shift
    tmux_cmd send-keys -t "$session" "$@"
}

# --- AGM Session Helpers ---
agm_session_exists() {
    # Check if session exists via agm session list (more reliable than tmux check)
    local session_name="$1"
    local output
    output=$(agm session list --no-color 2>&1)
    echo "$output" | grep -q "$session_name"
}

agm_session_is_active() {
    # Check session shows active indicator in list
    local session_name="$1"
    local output
    output=$(agm session list --no-color 2>&1)
    echo "$output" | grep -E "[●◐].*$session_name" >/dev/null 2>&1
}

agm_capture() {
    # Use agm capture command to get session pane content
    local session="$1"
    agm capture "$session" --no-color 2>/dev/null || echo ""
}

# --- Cleanup ---
cleanup_test_sessions() {
    # Archive all e2e test sessions via AGM
    local output
    output=$(agm session list --no-color 2>&1 || true)
    for name in $(echo "$output" | grep -oE 'e2e-[a-zA-Z0-9_-]+' | sort -u); do
        agm session kill "$name" 2>/dev/null || true
        agm session archive "$name" 2>/dev/null || true
    done
}

cleanup_all() {
    cleanup_test_sessions
}

# --- Setup ---
setup_e2e_env() {
    mkdir -p "$AGM_E2E_RESULTS_DIR"
    trap cleanup_all EXIT
}
