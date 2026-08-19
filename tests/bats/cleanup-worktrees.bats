#!/usr/bin/env bats
# Tests for scripts/cleanup-worktrees.sh — the shim in front of
# cmd/cleanup-worktrees.
#
# Scope note: this file covers only the shell contract that needs no Go
# toolchain, because the Bats matrix runs in bash/alpine/debian containers
# that install git, bash, and coreutils and nothing else. The shim's fallback
# path compiles the command with `go build`, so the guard behaviour itself
# (dirty, locked, and unmerged checkouts surviving a --fix run) is covered
# end-to-end through this same script by cmd/cleanup-worktrees/shim_test.go,
# which runs where a toolchain exists.
#
# What is testable here is the dispatch half, and it is worth testing: the
# shim decides whether to run a prebuilt binary or build one, and it has to
# forward arguments untouched and propagate the command's exit status.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    SCRIPT="$PROJECT_ROOT/scripts/cleanup-worktrees.sh"

    TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cleanup-worktrees-bats.XXXXXX")"

    # A throwaway copy of the repository layout the shim resolves against, so
    # planting a stub binary never touches the real bin/ directory.
    FAKE_ROOT="$TEST_DIR/root"
    mkdir -p "$FAKE_ROOT/scripts" "$FAKE_ROOT/bin"
    cp "$SCRIPT" "$FAKE_ROOT/scripts/cleanup-worktrees.sh"
    chmod +x "$FAKE_ROOT/scripts/cleanup-worktrees.sh"
    SHIM="$FAKE_ROOT/scripts/cleanup-worktrees.sh"
}

teardown() {
    rm -rf "$TEST_DIR"
}

# plant_stub writes a fake prebuilt binary that records its argv and exits
# with the given status.
plant_stub() {
    local status=$1
    cat > "$FAKE_ROOT/bin/cleanup-worktrees" <<STUB
#!/usr/bin/env bash
printf '%s\n' "\$@" > "$TEST_DIR/argv"
exit $status
STUB
    chmod +x "$FAKE_ROOT/bin/cleanup-worktrees"
}

@test "prefers a prebuilt binary over building one" {
    plant_stub 0
    run "$SHIM" /some/repo --fix
    assert_success
    # Nothing was compiled: a build would need a toolchain the container lacks.
    assert_file_exist "$TEST_DIR/argv"
}

@test "forwards every argument verbatim" {
    plant_stub 0
    run "$SHIM" /some/repo --fix --max-age 30 --preserve keep-me
    assert_success
    run cat "$TEST_DIR/argv"
    assert_line --index 0 "/some/repo"
    assert_line --index 1 "--fix"
    assert_line --index 2 "--max-age"
    assert_line --index 3 "30"
    assert_line --index 4 "--preserve"
    assert_line --index 5 "keep-me"
}

@test "preserves an argument containing spaces" {
    plant_stub 0
    run "$SHIM" "/repos/with space" --preserve "two words"
    assert_success
    run cat "$TEST_DIR/argv"
    assert_line --index 0 "/repos/with space"
    assert_line --index 2 "two words"
}

@test "propagates the command's exit status" {
    # Exit 3 is the tool's "a removal failed" signal. An automated caller has
    # to be able to see it through the shim.
    plant_stub 3
    run "$SHIM" /some/repo --fix
    assert_failure 3
}

@test "disables git terminal prompting" {
    cat > "$FAKE_ROOT/bin/cleanup-worktrees" <<STUB
#!/usr/bin/env bash
printf '%s' "\${GIT_TERMINAL_PROMPT-unset}" > "$TEST_DIR/prompt"
STUB
    chmod +x "$FAKE_ROOT/bin/cleanup-worktrees"
    run "$SHIM" /some/repo
    assert_success
    run cat "$TEST_DIR/prompt"
    assert_output "0"
}

@test "runs the binary from the caller's working directory" {
    # The shim resolves its own root, so it must not change the caller's cwd:
    # a relative <repo-path> has to keep meaning what the caller meant.
    cat > "$FAKE_ROOT/bin/cleanup-worktrees" <<STUB
#!/usr/bin/env bash
pwd > "$TEST_DIR/cwd"
STUB
    chmod +x "$FAKE_ROOT/bin/cleanup-worktrees"
    mkdir -p "$TEST_DIR/elsewhere"
    # Compare the resolved form: TMPDIR may carry a trailing slash, and pwd
    # normalizes it away.
    local want
    want="$(cd "$TEST_DIR/elsewhere" && pwd)"
    run bash -c "cd '$TEST_DIR/elsewhere' && '$SHIM' ."
    assert_success
    run cat "$TEST_DIR/cwd"
    assert_output "$want"
}
