#!/usr/bin/env bats
# Tests for scripts/gobin-guard.sh — SENSE + ESCALATE guard for ~/go/bin (ce-24f1).
#
# Each test stands up an isolated fake HOME with its own GOBIN dir and decision
# trail, then exercises one GOBIN state (healthy / missing dir / missing
# sentinel / non-executable sentinel) and asserts the exit code, the stderr
# alarm, and whether an escalation record was appended to the trail.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    SCRIPT="$PROJECT_ROOT/scripts/gobin-guard.sh"

    TEST_DIR="$(mktemp -d)"
    export FAKE_HOME="$TEST_DIR/home"
    export TRAIL="$TEST_DIR/trail.jsonl"
    mkdir -p "$FAKE_HOME"
	MOCK_BIN="$TEST_DIR/mock-bin"
	mkdir -p "$MOCK_BIN"
}

teardown() {
    rm -rf "$TEST_DIR"
}

# run_guard invokes the guard against the isolated fixture HOME/trail.
run_guard() {
    run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" "$SCRIPT" "$@"
}

trail_lines() {
    [ -f "$TRAIL" ] && wc -l <"$TRAIL" | tr -d ' ' || echo 0
}

@test "healthy GOBIN: exit 0, no escalation" {
    mkdir -p "$FAKE_HOME/go/bin"
    printf '#!/bin/sh\n' >"$FAKE_HOME/go/bin/agm"
    chmod +x "$FAKE_HOME/go/bin/agm"

    run_guard
    assert_success
    assert_output --partial "OK"
    assert_equal "$(trail_lines)" "0"
}

@test "healthy GOBIN --quiet: exit 0, no stdout" {
    mkdir -p "$FAKE_HOME/go/bin"
    printf '#!/bin/sh\n' >"$FAKE_HOME/go/bin/agm"
    chmod +x "$FAKE_HOME/go/bin/agm"

    run_guard --quiet
    assert_success
    assert_output ""
}

@test "missing GOBIN directory: exit 1, escalates" {
    # No go/bin at all — the exact 2026-07-15 failure mode.
    run_guard
    assert_failure 1
    assert_output --partial "missing_dir"
    assert_equal "$(trail_lines)" "1"
    assert_file_contains "$TRAIL" '"kind":"watchdog.gobin.missing"'
    assert_file_contains "$TRAIL" '"status":"missing_dir"'
}

@test "missing sentinel binary: exit 1, escalates" {
    mkdir -p "$FAKE_HOME/go/bin" # dir present but empty

    run_guard
    assert_failure 1
    assert_output --partial "missing_sentinel"
    assert_equal "$(trail_lines)" "1"
    assert_file_contains "$TRAIL" '"status":"missing_sentinel"'
}

@test "sentinel present but not executable: exit 1, escalates" {
    mkdir -p "$FAKE_HOME/go/bin"
    : >"$FAKE_HOME/go/bin/agm" # regular file, not +x

    run_guard
    assert_failure 1
    assert_output --partial "sentinel_not_executable"
    assert_equal "$(trail_lines)" "1"
}

@test "--json emits parseable status object on alarm" {
    run_guard --json
    assert_failure 1
    # Validate structural JSON markers without requiring python3/jq.
    assert_output --partial '"status":"missing_dir"'
    assert_output --partial '"gobin_dir":'
    assert_output --partial '"sentinel":'
    assert_output --partial '"reason":'
}

@test "custom sentinel via GOBIN_GUARD_BINARY" {
    mkdir -p "$FAKE_HOME/go/bin"
    printf '#!/bin/sh\n' >"$FAKE_HOME/go/bin/vroom-dispatch"
    chmod +x "$FAKE_HOME/go/bin/vroom-dispatch"

    run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" \
        GOBIN_GUARD_BINARY="vroom-dispatch" "$SCRIPT"
    assert_success
    assert_equal "$(trail_lines)" "0"
}

@test "escalation record is valid JSON with required decision-trail fields" {
    run_guard
    assert_failure 1
    # Validate required decision-trail fields using portable grep — no python3/jq needed.
    assert_file_contains "$TRAIL" '"kind":"watchdog.gobin.missing"'
    assert_file_contains "$TRAIL" '"role":"watchdog"'
    assert_file_contains "$TRAIL" '"event_id":'
    assert_file_contains "$TRAIL" '"timestamp":'
    assert_file_contains "$TRAIL" '"payload":'
    assert_file_contains "$TRAIL" '"status":"missing_dir"'
    # Timestamp must end with Z (UTC).
    run grep '"timestamp":"[^"]*Z"' "$TRAIL"
    assert_success
}

@test "macOS launchd alarm posts an active notification" {
	cat >"$MOCK_BIN/uname" <<'EOF'
#!/bin/sh
echo Darwin
EOF
	cat >"$MOCK_BIN/osascript" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$NOTIFY_LOG"
EOF
	chmod +x "$MOCK_BIN/uname" "$MOCK_BIN/osascript"
	export NOTIFY_LOG="$TEST_DIR/notifications.log"

	run env PATH="$MOCK_BIN:$PATH" HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" "$SCRIPT" --quiet
	assert_failure 1
	assert_file_contains "$NOTIFY_LOG" 'DEAR Agent GOBIN alarm'
}
