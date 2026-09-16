#!/usr/bin/env bash
# preflight-gate.sh — record which preflight gate failed, for callers to name.
#
# Sourced by scripts/preflight.sh. Kept separate so the recording logic is
# testable on its own (tests/bats/preflight-gate.bats) instead of only through
# a full preflight run.
#
# safe-pr sets PREFLIGHT_GATE_LOG and reads the file back when preflight exits
# non-zero. Without it a failure reaches the operator as "preflight-full
# failed ... exit status 1" with no gate and no test name, so diagnosing it
# costs a second full race suite (ce-2sgej).
#
# File format: first line is the gate name, any further lines are details.

# preflight_record_gate <gate> [detail ...]
# Writing the report is best-effort: an unwritable path must never convert a
# gate failure into a different failure.
preflight_record_gate() {
    local gate="$1"
    shift || true

    [ -n "${PREFLIGHT_GATE_LOG:-}" ] || return 0

    printf '%s\n' "$gate" >"$PREFLIGHT_GATE_LOG" 2>/dev/null || return 0
    if [ "$#" -gt 0 ]; then
        printf '%s\n' "$@" >>"$PREFLIGHT_GATE_LOG" 2>/dev/null || return 0
    fi
    return 0
}

# preflight_failing_tests <go-test-log>
# Prints the names of failing Go tests, one per line, deduplicated. Subtests
# are reported under their own name so the caller sees the exact case. A
# package that fails without a named test (build failure, panic, timeout) is
# reported by package path so the gate still points somewhere.
preflight_failing_tests() {
    local log="$1"
    [ -f "$log" ] || return 0

    {
        sed -n 's/^[[:space:]]*--- FAIL: \([^ ]*\).*/\1/p' "$log"
        sed -n 's/^FAIL[[:space:]]\{1,\}\([^[:space:]]\{1,\}\).*/\1/p' "$log"
    } | sed '/^$/d' | sort -u
}

# preflight_run_go_tests <gate> <log> <command...>
# Runs a Go test command, mirrors its output live, and on failure records the
# gate together with the names of the tests that failed. Returns the command's
# own success/failure so the caller decides how to abort.
preflight_run_go_tests() {
    local gate="$1"
    local log="$2"
    shift 2

    # Read the command's own status rather than the pipeline's. Without this
    # the result depends on the caller having set `pipefail`, and a caller
    # without it would read every failing suite as a pass.
    "$@" 2>&1 | tee "$log"
    local rc="${PIPESTATUS[0]}"
    if [ "$rc" -eq 0 ]; then
        return 0
    fi

    local failing=()
    local name
    while IFS= read -r name; do
        [ -n "$name" ] && failing+=("$name")
    done < <(preflight_failing_tests "$log")

    if [ "${#failing[@]}" -gt 0 ]; then
        preflight_record_gate "$gate" "${failing[@]}"
    else
        preflight_record_gate "$gate"
    fi
    return 1
}
