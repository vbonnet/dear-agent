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
    AUDIT_SCRIPT="$PROJECT_ROOT/scripts/gobin-guard-audit.sh"

    TEST_DIR="$(mktemp -d)"
    export FAKE_HOME="$TEST_DIR/home"
    export TRAIL="$TEST_DIR/trail.jsonl"
	export HEARTBEAT="$TEST_DIR/gobin-guard.heartbeat"
	export ALARM="$FAKE_HOME/.local/state/dear-agent/gobin-guard.alarm"
	export AUDIT_ALARM="$FAKE_HOME/.local/state/dear-agent/gobin-guard-audit.alarm"
    mkdir -p "$FAKE_HOME"
	MOCK_BIN="$TEST_DIR/mock-bin"
	mkdir -p "$MOCK_BIN"
}

teardown() {
    rm -rf "$TEST_DIR"
}

# run_guard invokes the guard against the isolated fixture HOME/trail.
run_guard() {
    run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" GOBIN_GUARD_ALARM_STATE="$ALARM" GOBIN_GUARD_NOTIFY=0 "$SCRIPT" "$@"
}

trail_lines() {
    [ -f "$TRAIL" ] && wc -l <"$TRAIL" | tr -d ' ' || echo 0
}

file_mode() {
	local mode
	if mode="$(stat -f '%Lp' "$1" 2>/dev/null)"; then
		printf '%s\n' "$mode"
	else
		stat -c '%a' "$1"
	fi
}

assert_fresh_private_alarm_tree() {
	for private_dir in "$FAKE_HOME/.local" "$FAKE_HOME/.local/state" "$FAKE_HOME/.local/state/dear-agent"; do
		assert_equal "$(file_mode "$private_dir")" "700"
	done
	assert_equal "$(file_mode "$1")" "600"
}

poison_alarm_tree() {
	mkdir -p "$FAKE_HOME/.local/state/dear-agent"
	chmod 600 "$FAKE_HOME/.local/state/dear-agent" "$FAKE_HOME/.local/state" "$FAKE_HOME/.local"
}

install_notification_mocks() {
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
}

notification_lines() {
	[ -f "$NOTIFY_LOG" ] && wc -l <"$NOTIFY_LOG" | tr -d ' ' || echo 0
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
	assert_file_exists "$HEARTBEAT"
}

@test "independent auditor alarms when guard heartbeat is missing" {
	poison_alarm_tree
	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" \
		GOBIN_GUARD_AUDIT_ALARM_STATE="$AUDIT_ALARM" GOBIN_GUARD_NOTIFY=0 /bin/sh "$AUDIT_SCRIPT"
	assert_failure 1
	assert_output --partial "heartbeat is missing or invalid"
	assert_file_contains "$TRAIL" '"kind":"watchdog.gobin_guard.stale"'
	assert_file_contains "$TRAIL" '"event_id":"gobin-guard-audit-'
	assert_fresh_private_alarm_tree "$AUDIT_ALARM"
	assert_equal "$(file_mode "$TRAIL")" "600"
	assert_equal "$(trail_lines)" "1"

	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" \
		GOBIN_GUARD_AUDIT_ALARM_STATE="$AUDIT_ALARM" GOBIN_GUARD_NOTIFY=0 /bin/sh "$AUDIT_SCRIPT"
	assert_failure 1
	assert_equal "$(trail_lines)" "1"
}

@test "independent auditor accepts a fresh guard heartbeat" {
	date +%s >"$HEARTBEAT"
	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" \
		GOBIN_GUARD_NOTIFY=0 /bin/sh "$AUDIT_SCRIPT"
	assert_success
}

@test "independent auditor rejects oversized heartbeat timestamps" {
	printf '999999999999999999999999\n' >"$HEARTBEAT"
	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" GOBIN_GUARD_NOTIFY=0 /bin/sh "$AUDIT_SCRIPT"
	assert_failure 1
	assert_output --partial "heartbeat is missing or invalid"
}

@test "missing GOBIN suppresses repeat escalation records" {
	run_guard
	assert_failure 1
	assert_equal "$(trail_lines)" "1"
	assert_fresh_private_alarm_tree "$ALARM"
	assert_equal "$(file_mode "$HEARTBEAT")" "600"
	assert_equal "$(file_mode "$TRAIL")" "600"
	run_guard
	assert_failure 1
	assert_equal "$(trail_lines)" "1"
}

@test "missing GOBIN repairs legacy non-searchable alarm parents" {
	poison_alarm_tree
	run_guard
	assert_failure 1
	assert_equal "$(trail_lines)" "1"
	assert_fresh_private_alarm_tree "$ALARM"
}

@test "default heartbeat and alarm preserve existing searchable parents" {
	mkdir -p "$FAKE_HOME/.local/state/dear-agent"
	chmod 755 "$FAKE_HOME/.local" "$FAKE_HOME/.local/state" "$FAKE_HOME/.local/state/dear-agent"
	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" GOBIN_GUARD_NOTIFY=0 "$SCRIPT" --quiet
	assert_failure 1
	for existing_dir in "$FAKE_HOME/.local" "$FAKE_HOME/.local/state" "$FAKE_HOME/.local/state/dear-agent"; do
		assert_equal "$(file_mode "$existing_dir")" "755"
	done
	assert_equal "$(file_mode "$ALARM")" "600"
	assert_equal "$(file_mode "$FAKE_HOME/.local/state/dear-agent/gobin-guard.heartbeat")" "600"
	assert_equal "$(file_mode "$TRAIL")" "600"
}

@test "notification-only guard delivery persists and suppresses its alarm" {
	install_notification_mocks
	trail_blocker="$TEST_DIR/trail-blocker"
	: >"$trail_blocker"
	bad_trail="$trail_blocker/trail.jsonl"

	for _ in 1 2; do
		run env PATH="$MOCK_BIN:$PATH" HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$bad_trail" \
			GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" GOBIN_GUARD_ALARM_STATE="$ALARM" \
			GOBIN_GUARD_NOTIFY=1 "$SCRIPT" --quiet
		assert_failure 1
	done

	assert_fresh_private_alarm_tree "$ALARM"
	assert_equal "$(notification_lines)" "1"
}

@test "notification-only auditor delivery persists and suppresses its alarm" {
	install_notification_mocks
	trail_blocker="$TEST_DIR/trail-blocker"
	: >"$trail_blocker"
	bad_trail="$trail_blocker/trail.jsonl"

	for _ in 1 2; do
		run env PATH="$MOCK_BIN:$PATH" HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$bad_trail" \
			GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" GOBIN_GUARD_AUDIT_ALARM_STATE="$AUDIT_ALARM" \
			GOBIN_GUARD_NOTIFY=1 /bin/sh "$AUDIT_SCRIPT"
		assert_failure 1
	done

	assert_fresh_private_alarm_tree "$AUDIT_ALARM"
	assert_equal "$(notification_lines)" "1"
}

@test "option-looking relative alarm paths persist for both publishers" {
	cd "$TEST_DIR"
	guard_trail="$TEST_DIR/guard-trail.jsonl"
	audit_trail="$TEST_DIR/audit-trail.jsonl"
	guard_alarm="-guard-state/a/alarm"
	audit_alarm="-audit-state/a/alarm"

	for _ in 1 2; do
		run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$guard_trail" GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" \
			GOBIN_GUARD_ALARM_STATE="$guard_alarm" GOBIN_GUARD_NOTIFY=0 "$SCRIPT" --quiet
		assert_failure 1
	done
	assert_equal "$(wc -l <"$guard_trail" | tr -d ' ')" "1"
	assert_equal "$(file_mode "$TEST_DIR/$guard_alarm")" "600"

	for _ in 1 2; do
		run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$audit_trail" GOBIN_GUARD_HEARTBEAT="$TEST_DIR/missing-heartbeat" \
			GOBIN_GUARD_AUDIT_ALARM_STATE="$audit_alarm" GOBIN_GUARD_NOTIFY=0 /bin/sh "$AUDIT_SCRIPT"
		assert_failure 1
	done
	assert_equal "$(wc -l <"$audit_trail" | tr -d ' ')" "1"
	assert_equal "$(file_mode "$TEST_DIR/$audit_alarm")" "600"
}

@test "alarm repair never chmods through a non-searchable symlink" {
	guard_target="$TEST_DIR/guard-target"
	audit_target="$TEST_DIR/audit-target"
	mkdir "$guard_target" "$audit_target"
	chmod 600 "$guard_target" "$audit_target"
	ln -s "$guard_target" "$FAKE_HOME/guard-link"
	ln -s "$audit_target" "$FAKE_HOME/audit-link"

	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TEST_DIR/guard-trail.jsonl" \
		GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" GOBIN_GUARD_ALARM_STATE="$FAKE_HOME/guard-link/state/alarm" \
		GOBIN_GUARD_NOTIFY=0 "$SCRIPT" --quiet
	assert_failure 1
	assert_equal "$(file_mode "$guard_target")" "600"

	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TEST_DIR/audit-trail.jsonl" \
		GOBIN_GUARD_HEARTBEAT="$TEST_DIR/missing-heartbeat" \
		GOBIN_GUARD_AUDIT_ALARM_STATE="$FAKE_HOME/audit-link/state/alarm" \
		GOBIN_GUARD_NOTIFY=0 /bin/sh "$AUDIT_SCRIPT"
	assert_failure 1
	assert_equal "$(file_mode "$audit_target")" "600"
}

@test "independent auditor rejects a multi-line heartbeat" {
	printf '1\n2\n' >"$HEARTBEAT"
	run env HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" GOBIN_GUARD_HEARTBEAT="$HEARTBEAT" GOBIN_GUARD_NOTIFY=0 /bin/sh "$AUDIT_SCRIPT"
	assert_failure 1
	assert_output --partial "heartbeat is missing or invalid"
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
	install_notification_mocks

	run env PATH="$MOCK_BIN:$PATH" HOME="$FAKE_HOME" GOBIN_GUARD_TRAIL="$TRAIL" "$SCRIPT" --quiet
	assert_failure 1
	assert_file_contains "$NOTIFY_LOG" 'DEAR Agent GOBIN alarm'
}
