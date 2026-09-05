#!/usr/bin/env bats
# Tests host-scoped full preflight advisory lease in scripts/preflight.sh.
#
# Verifies REPO-SCRIPT-09:
# 1. Full preflight acquires an exclusive lease and serializes concurrent runs.
# 2. Contention outputs owner diagnostics (PID, worktree, start time).
# 3. Cancellation (SIGINT/SIGTERM) reports owner without killing the leaseholder.
# 4. Timeout reports owner without killing the leaseholder.
# 5. Process death (SIGKILL) of leaseholder immediately unblocks the waiter.
# 6. Fast preflight does not acquire or contend on the lease.
# 7. Isolated preflight forwards host-scoped lease directory.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    SCRIPT="$PROJECT_ROOT/scripts/preflight.sh"
    ISOLATED_SCRIPT="$PROJECT_ROOT/scripts/isolated-preflight.sh"

    TEST_DIR="$(mktemp -d)"
    MOCK_BIN="$TEST_DIR/mock-bin"
    mkdir -p "$MOCK_BIN"
    export PREFLIGHT_LEASE_DIR="$TEST_DIR/lease"
    mkdir -p "$PREFLIGHT_LEASE_DIR"

    stub_toolchain 0
}

teardown() {
    # Ensure any lingering background jobs are killed
    if [[ -d "$TEST_DIR" ]]; then
        rm -rf "$TEST_DIR"
    fi
}

stub_toolchain() {
    local sleep_sec="${1:-0}"
    for tool in go make golangci-lint govulncheck jq; do
        cat >"$MOCK_BIN/$tool" <<EOF
#!/bin/sh
if [ "$tool" = "golangci-lint" ] && [ "\${1:-}" = "version" ]; then
    echo "stub golangci-lint"
    exit 0
fi
if [ "$tool" = "govulncheck" ]; then
    exit 0
fi
if [ "$sleep_sec" -gt 0 ] && [ "$tool" = "go" ] && [ "\${1:-}" = "mod" ]; then
    sleep $sleep_sec
fi
exit 0
EOF
        chmod +x "$MOCK_BIN/$tool"
    done
}

hold_lease_background() {
    local lock_file="$1"
    local duration="$2"
    if command -v lockf >/dev/null 2>&1; then
        lockf -k "$lock_file" sleep "$duration" &
    elif flock --help 2>&1 | grep -q -- '-w'; then
        flock -x "$lock_file" sleep "$duration" &
    else
        flock "$lock_file" sleep "$duration" &
    fi
}

@test "full preflight acquires lease and serializes concurrent runs" {
    # Stub toolchain so worker 1 holds lease for ~1s
    stub_toolchain 1

    local log1="$TEST_DIR/w1.log"
    local log2="$TEST_DIR/w2.log"
    local time_file="$TEST_DIR/times.log"

    (
        env PATH="$MOCK_BIN:$PATH" PREFLIGHT_LEASE_DIR="$PREFLIGHT_LEASE_DIR" "$SCRIPT" --full >"$log1" 2>&1
        date +%s >> "$time_file"
    ) &
    local pid1=$!

    # Give worker 1 time to acquire the lease and enter work
    sleep 0.5

    (
        env PATH="$MOCK_BIN:$PATH" PREFLIGHT_LEASE_DIR="$PREFLIGHT_LEASE_DIR" "$SCRIPT" --full >"$log2" 2>&1
        date +%s >> "$time_file"
    ) &
    local pid2=$!

    wait "$pid1"
    wait "$pid2"

    assert_equal "$?" 0
    [ -f "$log1" ]
    [ -f "$log2" ]
    grep -q "preflight full passed" "$log1"
    grep -q "preflight full passed" "$log2"

    # Worker 2 should report waiting for worker 1
    grep -q "waiting for full-preflight lease" "$log2"
}

@test "contended full preflight outputs owner diagnostics" {
    # Create fake owner metadata and hold the lock
    local lock_file="$PREFLIGHT_LEASE_DIR/preflight-full.lock"
    local owner_file="$PREFLIGHT_LEASE_DIR/preflight-full.owner"
    touch "$lock_file"

    cat > "$owner_file" <<EOF
PID=99999
STARTED=2026-09-04T12:00:00Z
WORKTREE=/fake/worktree
COMMAND=scripts/preflight.sh --full
EOF

    # Hold the advisory lock
    hold_lease_background "$lock_file" 2
    local holder_pid=$!
    sleep 0.5

    run env PATH="$MOCK_BIN:$PATH" PREFLIGHT_LEASE_DIR="$PREFLIGHT_LEASE_DIR" PREFLIGHT_LEASE_TIMEOUT=1 "$SCRIPT" --full

    wait "$holder_pid" || true

    # Should have timed out and printed owner diagnostics
    assert_failure
    [[ "$output" == *"waiting for full-preflight lease held by PID 99999 (/fake/worktree) since 2026-09-04T12:00:00Z"* ]]
    [[ "$output" == *"timed out waiting for full-preflight lease after 1s"* ]]
}

@test "cancellation while waiting reports owner without killing leaseholder" {
    local lock_file="$PREFLIGHT_LEASE_DIR/preflight-full.lock"
    local owner_file="$PREFLIGHT_LEASE_DIR/preflight-full.owner"
    touch "$lock_file"

    cat > "$owner_file" <<EOF
PID=88888
STARTED=2026-09-04T12:00:00Z
WORKTREE=/holder/worktree
COMMAND=scripts/preflight.sh --full
EOF

    hold_lease_background "$lock_file" 15
    local holder_pid=$!
    sleep 0.5

    local waiter_log="$TEST_DIR/waiter.log"
    env PATH="$MOCK_BIN:$PATH" PREFLIGHT_LEASE_DIR="$PREFLIGHT_LEASE_DIR" PREFLIGHT_LEASE_TIMEOUT=10 "$SCRIPT" --full >"$waiter_log" 2>&1 &
    local waiter_pid=$!

    sleep 0.5
    kill -TERM "$waiter_pid"
    wait "$waiter_pid" || true

    # Assert holder is still alive!
    kill -0 "$holder_pid" 2>/dev/null
    assert_equal "$?" 0

    kill -9 "$holder_pid" 2>/dev/null || true
    wait "$holder_pid" 2>/dev/null || true

    [ -f "$waiter_log" ]
    cat "$waiter_log"
    grep -q "cancelled while waiting for full-preflight lease (held by PID 88888 (/holder/worktree) since 2026-09-04T12:00:00Z)" "$waiter_log"
}

@test "process death of leaseholder releases lease immediately for waiter" {
    local lock_file="$PREFLIGHT_LEASE_DIR/preflight-full.lock"
    local owner_file="$PREFLIGHT_LEASE_DIR/preflight-full.owner"
    touch "$lock_file"

    hold_lease_background "$lock_file" 20
    local holder_pid=$!
    sleep 0.5

    local waiter_log="$TEST_DIR/waiter-death.log"
    env PATH="$MOCK_BIN:$PATH" PREFLIGHT_LEASE_DIR="$PREFLIGHT_LEASE_DIR" PREFLIGHT_LEASE_TIMEOUT=5 "$SCRIPT" --full >"$waiter_log" 2>&1 &
    local waiter_pid=$!

    sleep 0.5
    # Abruptly kill the holder
    kill -9 "$holder_pid"
    wait "$holder_pid" 2>/dev/null || true

    # Waiter should acquire lease and finish successfully
    wait "$waiter_pid"
    assert_equal "$?" 0

    [ -f "$waiter_log" ]
    grep -q "preflight full passed" "$waiter_log"
}

@test "fast preflight does not acquire or block on lease" {
    local lock_file="$PREFLIGHT_LEASE_DIR/preflight-full.lock"
    touch "$lock_file"

    hold_lease_background "$lock_file" 3
    local holder_pid=$!
    sleep 0.5

    # Fast preflight should succeed immediately without waiting
    run env PATH="$MOCK_BIN:$PATH" PREFLIGHT_LEASE_DIR="$PREFLIGHT_LEASE_DIR" "$SCRIPT" --fast
    assert_success
    [[ "$output" == *"preflight fast passed"* ]]
    [[ "$output" != *"waiting for full-preflight lease"* ]]

    wait "$holder_pid"
}

@test "isolated preflight forwards host-scoped lease directory" {
    local custom_lease_dir="$TEST_DIR/custom-isolated-lease"
    mkdir -p "$custom_lease_dir"

    # Verify that PREFLIGHT_LEASE_DIR is forwarded to preflight.sh
    cat >"$MOCK_BIN/test-probe" <<'EOF'
#!/usr/bin/env bash
echo "LEASE_DIR=$PREFLIGHT_LEASE_DIR"
exit 0
EOF
    chmod +x "$MOCK_BIN/test-probe"

    # Create mock repo
    local mock_repo="$TEST_DIR/mock-repo"
    mkdir -p "$mock_repo/scripts"
    cp "$ISOLATED_SCRIPT" "$mock_repo/scripts/isolated-preflight.sh"
    chmod +x "$mock_repo/scripts/isolated-preflight.sh"

    cat >"$mock_repo/scripts/preflight.sh" <<'EOF'
#!/usr/bin/env bash
echo "RECEIVED_LEASE_DIR=${PREFLIGHT_LEASE_DIR:-unset}"
exit 0
EOF
    chmod +x "$mock_repo/scripts/preflight.sh"

    run env PREFLIGHT_LEASE_DIR="$custom_lease_dir" PREFLIGHT_TMP_DIR="$TEST_DIR/tmp" "$mock_repo/scripts/isolated-preflight.sh" --full
    assert_success
    [[ "$output" == *"RECEIVED_LEASE_DIR=$custom_lease_dir"* ]]
}
