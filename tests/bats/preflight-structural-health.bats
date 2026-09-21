#!/usr/bin/env bats
# Tests that every local preflight tier runs the authoritative structural-health
# scanner once, and that a rejected scan is a named gate before repository tests.
# Verifies REPO-SCRIPT-11.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"

    TEST_DIR="$(mktemp -d)"
    TEST_REPO="$TEST_DIR/repo"
    MOCK_BIN="$TEST_DIR/mock-bin"
    SCRIPT="$TEST_REPO/scripts/preflight.sh"
    export PREFLIGHT_COMMAND_LOG="$TEST_DIR/commands.log"
    export PREFLIGHT_GATE_LOG="$TEST_DIR/gate.log"
    export PREFLIGHT_LEASE_DIR="$TEST_DIR/lease"
    mkdir -p "$MOCK_BIN" "$PREFLIGHT_LEASE_DIR" "$TEST_REPO/scripts/lib"
    cp -f "$PROJECT_ROOT/scripts/preflight.sh" "$SCRIPT"
    cp -f "$PROJECT_ROOT/scripts/lib/preflight-gate.sh" "$TEST_REPO/scripts/lib/preflight-gate.sh"

    stub_toolchain
}

teardown() {
    rm -rf "$TEST_DIR"
}

stub_toolchain() {
    cat >"$MOCK_BIN/go" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$PREFLIGHT_COMMAND_LOG"
if [[ "$#" -eq 2 && "$1" == "build" && "$2" == "./..." && "${BUILD_EXIT:-0}" -ne 0 ]]; then
    echo "synthetic build failure" >&2
    exit "$BUILD_EXIT"
fi
if [[ "$#" -eq 2 && "$1" == "run" && "$2" == "./cmd/structural-health" ]]; then
    if [[ "${STRUCTURAL_HEALTH_EXIT:-0}" -ne 0 ]]; then
        echo "synthetic structural regression" >&2
    fi
    exit "${STRUCTURAL_HEALTH_EXIT:-0}"
fi
exit 0
EOF

    cat >"$MOCK_BIN/golangci-lint" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "version" ]]; then
    echo "stub golangci-lint"
fi
exit 0
EOF

    # Exercise full preflight's outer/inner re-exec shape without depending on
    # the host's lockf/flock implementation. This adapter consumes lockf's
    # options and lock-file operand, then executes the protected command.
    cat >"$MOCK_BIN/lockf" <<'EOF'
#!/usr/bin/env bash
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        -s|-k) shift ;;
        -t) shift 2 ;;
        *) break ;;
    esac
done
shift
exec "$@"
EOF

    for tool in make govulncheck jq; do
        cat >"$MOCK_BIN/$tool" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
    done

    chmod +x "$MOCK_BIN"/*
}

structural_health_invocations() {
    awk '$0 == "run ./cmd/structural-health" { count++ } END { print count + 0 }' \
        "$PREFLIGHT_COMMAND_LOG" 2>/dev/null
}

@test "every preflight tier invokes structural health exactly once" {
    local mode
    for mode in --fast --tests --race --full; do
        : >"$PREFLIGHT_COMMAND_LOG"

        run env PATH="$MOCK_BIN:$PATH" "$SCRIPT" "$mode"

        assert_success "$mode failed: $output"
        assert_equal "$(structural_health_invocations)" "1" "$mode invocation count"
    done
}

@test "structural health does not run when compilation fails" {
    run env PATH="$MOCK_BIN:$PATH" BUILD_EXIT=1 "$SCRIPT" --fast

    assert_failure
    assert_output --partial "synthetic build failure"
    assert_equal "$(structural_health_invocations)" "0"
}

@test "a structural health regression is named and stops before repository tests" {
    run env PATH="$MOCK_BIN:$PATH" STRUCTURAL_HEALTH_EXIT=1 "$SCRIPT" --full

    assert_failure
    assert_output --partial "synthetic structural regression"
    assert_output --partial "structural-health scan failed"
    assert_equal "$(head -n 1 "$PREFLIGHT_GATE_LOG")" "structural-health scan failed"
    assert_equal "$(structural_health_invocations)" "1"
    run grep -E '^test([[:space:]]|$)' "$PREFLIGHT_COMMAND_LOG"
    assert_failure
}
