#!/usr/bin/env bats
# Tests scripts/isolated-preflight.sh.
#
# Verifies that:
# 1. Scratch directories are provisioned under PREFLIGHT_TMP_DIR (or default).
# 2. On successful execution (exit 0), the scratch directory is removed by trap.
# 3. On failing execution (exit 1), the scratch directory is still removed by trap.
# 4. An explicit PREFLIGHT_TMP_DIR is honored.
# 5. Minimal git configuration is forwarded when present.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    SCRIPT="$PROJECT_ROOT/scripts/isolated-preflight.sh"

    TEST_DIR="$(mktemp -d)"
    MOCK_PREFLIGHT_ROOT="$TEST_DIR/mock-repo"
    MOCK_TMP_PARENT="$TEST_DIR/preflight-parent"
    mkdir -p "$MOCK_PREFLIGHT_ROOT/scripts" "$MOCK_TMP_PARENT"

    # Copy script under mock root
    cp "$SCRIPT" "$MOCK_PREFLIGHT_ROOT/scripts/isolated-preflight.sh"
    chmod +x "$MOCK_PREFLIGHT_ROOT/scripts/isolated-preflight.sh"
}

teardown() {
    rm -rf "$TEST_DIR"
}

stub_inner_preflight() {
    local exit_code="$1"
    local record_env="$2"
    cat >"$MOCK_PREFLIGHT_ROOT/scripts/preflight.sh" <<INNER_EOF
#!/usr/bin/env bash
if [ -n "$record_env" ]; then
    printf 'HOME=%s\nGOCACHE=%s\nTMPDIR=%s\n' "\$HOME" "\$GOCACHE" "\$TMPDIR" >"$record_env"
fi
exit $exit_code
INNER_EOF
    chmod +x "$MOCK_PREFLIGHT_ROOT/scripts/preflight.sh"
}

@test "isolated preflight cleans up scratch directory on success" {
    env_log="$TEST_DIR/env.log"
    stub_inner_preflight 0 "$env_log"

    run env PREFLIGHT_TMP_DIR="$MOCK_TMP_PARENT" "$MOCK_PREFLIGHT_ROOT/scripts/isolated-preflight.sh" --fast
    assert_success

    [ -f "$env_log" ]
    recorded_home="$(grep '^HOME=' "$env_log" | cut -d= -f2)"
    [ -n "$recorded_home" ]
    run_dir="$(dirname "$recorded_home")"

    # The run dir must have been beneath MOCK_TMP_PARENT
    [[ "$run_dir" == "$MOCK_TMP_PARENT"* ]]
    # The run dir must no longer exist on disk
    [ ! -d "$run_dir" ]
}

@test "isolated preflight cleans up scratch directory on failure" {
    env_log="$TEST_DIR/env.log"
    stub_inner_preflight 1 "$env_log"

    run env PREFLIGHT_TMP_DIR="$MOCK_TMP_PARENT" "$MOCK_PREFLIGHT_ROOT/scripts/isolated-preflight.sh" --fast
    assert_failure
    [ "$status" -eq 1 ]

    [ -f "$env_log" ]
    recorded_home="$(grep '^HOME=' "$env_log" | cut -d= -f2)"
    [ -n "$recorded_home" ]
    run_dir="$(dirname "$recorded_home")"

    # The run dir must no longer exist on disk despite exit 1
    [ ! -d "$run_dir" ]
}

@test "isolated preflight isolates HOME, GOCACHE, and TMPDIR" {
    env_log="$TEST_DIR/env.log"
    stub_inner_preflight 0 "$env_log"

    run env PREFLIGHT_TMP_DIR="$MOCK_TMP_PARENT" "$MOCK_PREFLIGHT_ROOT/scripts/isolated-preflight.sh"
    assert_success

    recorded_home="$(grep '^HOME=' "$env_log" | cut -d= -f2)"
    recorded_gocache="$(grep '^GOCACHE=' "$env_log" | cut -d= -f2)"
    recorded_tmp="$(grep '^TMPDIR=' "$env_log" | cut -d= -f2)"

    [[ "$recorded_home" == "$MOCK_TMP_PARENT"/*/home ]]
    [[ "$recorded_gocache" == "$MOCK_TMP_PARENT"/*/gocache ]]
    [[ "$recorded_tmp" == "$MOCK_TMP_PARENT"/*/tmp ]]
}

@test "isolated preflight forwards host gitconfig when present" {
    fake_home="$TEST_DIR/fake-home"
    mkdir -p "$fake_home"
    cat >"$fake_home/.gitconfig" <<INNER_EOF
[user]
    name = Test User
    email = test@example.com
INNER_EOF

    cat >"$MOCK_PREFLIGHT_ROOT/scripts/preflight.sh" <<'INNER_EOF'
#!/usr/bin/env bash
if [ -f "$HOME/.gitconfig" ] && grep -q "Test User" "$HOME/.gitconfig"; then
    exit 0
fi
exit 3
INNER_EOF
    chmod +x "$MOCK_PREFLIGHT_ROOT/scripts/preflight.sh"

    run env HOME="$fake_home" PREFLIGHT_TMP_DIR="$MOCK_TMP_PARENT" "$MOCK_PREFLIGHT_ROOT/scripts/isolated-preflight.sh"
    assert_success
}

@test "isolated preflight cleans up scratch directory containing read-only module caches" {
    cat >"$MOCK_PREFLIGHT_ROOT/scripts/preflight.sh" <<'INNER_EOF'
#!/usr/bin/env bash
mkdir -p "$HOME/go/pkg/mod/cache/download"
touch "$HOME/go/pkg/mod/cache/download/sample"
chmod 0444 "$HOME/go/pkg/mod/cache/download/sample"
chmod 0555 "$HOME/go/pkg/mod/cache/download"
chmod 0555 "$HOME/go/pkg/mod/cache"
chmod 0555 "$HOME/go/pkg/mod"
echo "$HOME" > "$TMPDIR/recorded_home"
exit 0
INNER_EOF
    chmod +x "$MOCK_PREFLIGHT_ROOT/scripts/preflight.sh"

    run env PREFLIGHT_TMP_DIR="$MOCK_TMP_PARENT" "$MOCK_PREFLIGHT_ROOT/scripts/isolated-preflight.sh"
    assert_success

    # Parent directory should be completely empty (no leftover run.XXXXXX)
    run_dirs=("$MOCK_TMP_PARENT"/run.*)
    [ ! -e "${run_dirs[0]}" ]
}
