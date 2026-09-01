#!/usr/bin/env bats
# Tests the golangci-lint invocation in scripts/preflight.sh.
#
# golangci-lint takes a file lock at $TMPDIR/golangci-lint.lock on every run and
# exits 3 with "parallel golangci-lint is running" when it cannot acquire it.
# That lock is global to the host: it is scoped to neither the repository nor
# the checkout. safe-pr's own transaction lock is keyed on the worktree git dir,
# so it does NOT serialize two agents working in different worktrees; both reach
# `make preflight-full`, and one dies on the other's linter lock.
#
# The collision is also invisible. preflight.sh collapsed the linter's exit 3
# into fail()'s exit 1, so safe-pr's audit log recorded it as an ordinary
# "preflight-full failed ... exit status 1", indistinguishable from a real lint
# failure. 123 such records exist with no way to tell the two apart.
#
# GOLANGCI_LINT_CACHE alone does NOT fix this: the lock lives under $TMPDIR, not
# under the cache, so isolating only the cache leaves both runs dying. Releasing
# the lock needs --allow-parallel-runners, and running genuinely in parallel is
# only safe when each run also owns its cache. Both halves are required.
#
# These tests stub the toolchain and assert the contract of the invocation. The
# wall-clock concurrency proof is a manual two-worktree run, not a CI test.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    SCRIPT="$PROJECT_ROOT/scripts/preflight.sh"

    TEST_DIR="$(mktemp -d)"
    MOCK_BIN="$TEST_DIR/mock-bin"
    export LINT_ARGV="$TEST_DIR/argv.log"
    export LINT_CACHE_SEEN="$TEST_DIR/cache.log"
    mkdir -p "$MOCK_BIN"

    stub_linter 0
    stub_toolchain
}

teardown() {
    rm -rf "$TEST_DIR"
}

# stub_linter installs a golangci-lint that records how it was called and then
# exits with the given code.
stub_linter() {
    cat >"$MOCK_BIN/golangci-lint" <<EOF
#!/usr/bin/env bash
if [ "\$1" = "version" ]; then echo "stub golangci-lint"; exit 0; fi
printf '%s\n' "\$*" >>"$LINT_ARGV"
printf '%s\n' "\${GOLANGCI_LINT_CACHE:-<unset>}" >>"$LINT_CACHE_SEEN"
if [ "$1" = "3" ]; then echo "Error: parallel golangci-lint is running" >&2; fi
exit $1
EOF
    chmod +x "$MOCK_BIN/golangci-lint"
}

# stub_toolchain neutralises the surrounding gates so a test exercises only the
# lint step. go and make are the only other commands the fast tier shells out to.
stub_toolchain() {
    for tool in go make; do
        cat >"$MOCK_BIN/$tool" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
        chmod +x "$MOCK_BIN/$tool"
    done
}

run_preflight() {
    run env PATH="$MOCK_BIN:$PATH" "$SCRIPT" --fast
}

lint_argv() { cat "$LINT_ARGV" 2>/dev/null || true; }
lint_cache() { cat "$LINT_CACHE_SEEN" 2>/dev/null || true; }

@test "lint releases the global lock with --allow-parallel-runners" {
    run_preflight
    assert_success
    [[ "$(lint_argv)" == *"--allow-parallel-runners"* ]]
}

@test "lint still runs against the whole module with a timeout" {
    run_preflight
    [[ "$(lint_argv)" == *"./..."* ]]
    [[ "$(lint_argv)" == *"--timeout"* ]]
}

@test "lint gets an explicit GOLANGCI_LINT_CACHE" {
    run_preflight
    [ "$(lint_cache)" != "<unset>" ]
    [ -n "$(lint_cache)" ]
}

@test "the lint cache is scoped to this checkout, not shared host-wide" {
    run_preflight
    cache="$(lint_cache)"
    # Parallel runners over one shared cache is the corruption case golangci-lint
    # warns about, so the cache must not be either default location.
    [ "$cache" != "$HOME/Library/Caches/golangci-lint" ]
    [ "$cache" != "$HOME/.cache/golangci-lint" ]
    # It must be derived from this checkout so two worktrees never share one.
    [[ "$cache" == *"$(printf '%s' "$PROJECT_ROOT" | cksum | cut -d' ' -f1)"* ]]
}

@test "an explicit GOLANGCI_LINT_CACHE from the caller is honoured" {
    run env PATH="$MOCK_BIN:$PATH" GOLANGCI_LINT_CACHE="$TEST_DIR/caller-cache" "$SCRIPT" --fast
    [ "$(lint_cache)" = "$TEST_DIR/caller-cache" ]
}

@test "a lock collision is reported distinctly, not as a generic lint failure" {
    # Exit 3 is golangci-lint's documented "could not acquire lock" code. Even
    # with the lock released it must never again be laundered into the same
    # generic message a real lint failure produces.
    stub_linter 3
    run_preflight
    assert_failure
    assert_output --partial "could not acquire its lock"
}

@test "an ordinary lint failure is still reported as a lint failure" {
    stub_linter 1
    run_preflight
    assert_failure
    assert_output --partial "lint failed"
    refute_output --partial "could not acquire its lock"
}
