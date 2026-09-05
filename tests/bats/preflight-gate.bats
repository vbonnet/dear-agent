#!/usr/bin/env bats
# Tests for scripts/lib/preflight-gate.sh — the failing-gate report safe-pr
# reads so a preflight failure names its gate instead of a bare exit status
# (ce-2sgej).

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    TEST_DIR="$(mktemp -d)"

    # shellcheck source=/dev/null
    source "$PROJECT_ROOT/scripts/lib/preflight-gate.sh"
}

teardown() {
    rm -rf "$TEST_DIR"
}

@test "records the gate name as the first line" {
    export PREFLIGHT_GATE_LOG="$TEST_DIR/gate"
    preflight_record_gate "go vet failed"

    run head -n 1 "$PREFLIGHT_GATE_LOG"
    assert_success
    assert_output "go vet failed"
}

@test "records detail lines after the gate name" {
    export PREFLIGHT_GATE_LOG="$TEST_DIR/gate"
    preflight_record_gate "tests failed" "TestOne" "TestTwo"

    run cat "$PREFLIGHT_GATE_LOG"
    assert_success
    assert_line --index 0 "tests failed"
    assert_line --index 1 "TestOne"
    assert_line --index 2 "TestTwo"
}

@test "overwrites a previous report rather than appending" {
    export PREFLIGHT_GATE_LOG="$TEST_DIR/gate"
    preflight_record_gate "lint failed" "stale detail"
    preflight_record_gate "tests failed"

    run cat "$PREFLIGHT_GATE_LOG"
    assert_success
    assert_output "tests failed"
}

@test "is a no-op when PREFLIGHT_GATE_LOG is unset" {
    unset PREFLIGHT_GATE_LOG
    run preflight_record_gate "go vet failed"
    assert_success
}

@test "an unwritable report path does not fail the caller" {
    export PREFLIGHT_GATE_LOG="$TEST_DIR/does/not/exist/gate"
    run preflight_record_gate "go vet failed"
    assert_success
}

@test "extracts failing test names from a go test log" {
    cat >"$TEST_DIR/test.log" <<'LOG'
ok  	github.com/example/passing	0.2s
--- FAIL: TestRepositoryTreeHasNoTemporalDebt (0.21s)
    main_test.go:222: tracked temporal artifact: WAYFINDER-STATUS.md
    --- FAIL: TestOuter/subcase (0.01s)
FAIL	github.com/example/routing-guard	0.46s
LOG

    run preflight_failing_tests "$TEST_DIR/test.log"
    assert_success
    assert_line "TestRepositoryTreeHasNoTemporalDebt"
    assert_line "TestOuter/subcase"
    assert_line "github.com/example/routing-guard"
    refute_line --partial "passing"
}

@test "deduplicates repeated failures" {
    cat >"$TEST_DIR/test.log" <<'LOG'
--- FAIL: TestFlaky (0.01s)
--- FAIL: TestFlaky (0.01s)
LOG

    run preflight_failing_tests "$TEST_DIR/test.log"
    assert_success
    assert_output "TestFlaky"
}

@test "reports nothing for a missing log" {
    run preflight_failing_tests "$TEST_DIR/absent.log"
    assert_success
    assert_output ""
}

# The runner must not depend on the caller having enabled `pipefail`: this
# suite deliberately runs without it, so a regression to a bare pipeline exit
# status shows up as a failing suite reported as a pass.
@test "runner mirrors output and returns success when tests pass" {
    export PREFLIGHT_GATE_LOG="$TEST_DIR/gate"
    run preflight_run_go_tests "tests failed" "$TEST_DIR/run.log" \
        printf 'ok  github.com/example/pkg\n'

    assert_success
    assert_output --partial "ok  github.com/example/pkg"
    [ ! -f "$PREFLIGHT_GATE_LOG" ]
}

@test "runner records the gate and failing tests when the command fails" {
    export PREFLIGHT_GATE_LOG="$TEST_DIR/gate"
    cat >"$TEST_DIR/failing-suite" <<'STUB'
#!/usr/bin/env bash
echo "--- FAIL: TestRepositoryTreeHasNoTemporalDebt (0.21s)"
echo "FAIL	github.com/example/routing-guard	0.46s"
exit 1
STUB
    chmod +x "$TEST_DIR/failing-suite"

    run preflight_run_go_tests "tests failed" "$TEST_DIR/run.log" "$TEST_DIR/failing-suite"
    assert_failure

    run cat "$PREFLIGHT_GATE_LOG"
    assert_line --index 0 "tests failed"
    assert_line "TestRepositoryTreeHasNoTemporalDebt"
    assert_line "github.com/example/routing-guard"
}

@test "runner records the bare gate when no test name is parseable" {
    export PREFLIGHT_GATE_LOG="$TEST_DIR/gate"
    cat >"$TEST_DIR/silent-failure" <<'STUB'
#!/usr/bin/env bash
echo "build constraints exclude all Go files"
exit 2
STUB
    chmod +x "$TEST_DIR/silent-failure"

    run preflight_run_go_tests "tests failed" "$TEST_DIR/run.log" "$TEST_DIR/silent-failure"
    assert_failure

    run cat "$PREFLIGHT_GATE_LOG"
    assert_output "tests failed"
}

# A clean log has no failing tests. Under `set -o pipefail` the name-extraction
# pipeline must still succeed: a filter that exits non-zero when it matches
# nothing would make the healthy case look like a gate failure.
@test "preflight_failing_tests succeeds on a log with no failures" {
    local log="$BATS_TEST_TMPDIR/clean.log"
    printf 'ok  \tgithub.com/x/y\t0.10s\n' > "$log"

    run bash -c "set -o pipefail; source '$PROJECT_ROOT/scripts/lib/preflight-gate.sh'; preflight_failing_tests '$log'"

    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "preflight_failing_tests drops empty names from malformed FAIL lines" {
    local log="$BATS_TEST_TMPDIR/malformed.log"
    printf '    --- FAIL: TestReal (0.01s)\n    --- FAIL:  \n' > "$log"

    run bash -c "set -o pipefail; source '$PROJECT_ROOT/scripts/lib/preflight-gate.sh'; preflight_failing_tests '$log'"

    [ "$status" -eq 0 ]
    [ "$output" = "TestReal" ]
}
