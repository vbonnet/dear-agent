#!/usr/bin/env bats
# Tests for scripts/codegraph — the graphify wrapper.
#
# These tests exercise the script's argument parsing and path-resolution
# logic. They do NOT actually invoke graphify (which is a heavy Python
# dependency); instead they redirect CODEGRAPH_VENV to a stub that
# records its arguments, so we can verify the wrapper builds the right
# command without paying for a real graph build.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    TEST_DIR="$(mktemp -d)"
    export HOME="$TEST_DIR"

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    SCRIPT="$PROJECT_ROOT/scripts/codegraph"

    # Stub graphify venv that records args + env to a log file.
    STUB_VENV="$TEST_DIR/venv"
    mkdir -p "$STUB_VENV/bin"
    cat > "$STUB_VENV/bin/graphify" <<'EOF'
#!/usr/bin/env bash
echo "ARGS: $*" >> "$GRAPHIFY_STUB_LOG"
echo "GRAPHIFY_OUT=$GRAPHIFY_OUT" >> "$GRAPHIFY_STUB_LOG"
echo "[stub] would have run graphify $*"
EOF
    chmod +x "$STUB_VENV/bin/graphify"

    export GRAPHIFY_STUB_LOG="$TEST_DIR/stub.log"
    export CODEGRAPH_VENV="$STUB_VENV"
    export CODEGRAPH_OUT_BASE="$TEST_DIR/out"

    # A fake repo so repo_name() resolves something deterministic.
    REPO="$TEST_DIR/myrepo"
    mkdir -p "$REPO"
    git -C "$REPO" init -q
    git -C "$REPO" remote add origin https://example.com/example/myrepo.git
}

teardown() {
    rm -rf "$TEST_DIR"
}

@test "codegraph script is executable" {
    assert_file_executable "$SCRIPT"
}

@test "codegraph script uses set -euo pipefail" {
    run head -25 "$SCRIPT"
    assert_output --partial "set -euo pipefail"
}

@test "help prints the usage banner" {
    run "$SCRIPT" help
    assert_success
    assert_output --partial "Usage:"
    assert_output --partial "scripts/codegraph"
    assert_output --partial "CODEGRAPH_VENV"
}

@test "where prints derived output dir for a git repo" {
    run "$SCRIPT" where "$REPO"
    assert_success
    assert_output "$CODEGRAPH_OUT_BASE/myrepo"
}

@test "where falls back to basename outside a git repo" {
    plain="$TEST_DIR/not-a-repo"
    mkdir -p "$plain"
    run "$SCRIPT" where "$plain"
    assert_success
    assert_output "$CODEGRAPH_OUT_BASE/not-a-repo"
}

@test "build invokes graphify update with GRAPHIFY_OUT set to per-repo dir" {
    run "$SCRIPT" "$REPO"
    assert_success
    assert_file_exists "$GRAPHIFY_STUB_LOG"
    run cat "$GRAPHIFY_STUB_LOG"
    assert_output --partial "ARGS: update $REPO"
    assert_output --partial "GRAPHIFY_OUT=$CODEGRAPH_OUT_BASE/myrepo"
}

@test "build creates the per-repo output directory" {
    "$SCRIPT" "$REPO" >/dev/null
    assert_dir_exists "$CODEGRAPH_OUT_BASE/myrepo"
}

@test "query forwards --graph pointing at the repo's graph.json" {
    run "$SCRIPT" query "what is foo?" "$REPO"
    assert_success
    run cat "$GRAPHIFY_STUB_LOG"
    assert_output --partial "ARGS: query what is foo? --graph $CODEGRAPH_OUT_BASE/myrepo/graph.json"
}

@test "explain forwards --graph pointing at the repo's graph.json" {
    run "$SCRIPT" explain "Foo" "$REPO"
    assert_success
    run cat "$GRAPHIFY_STUB_LOG"
    assert_output --partial "ARGS: explain Foo --graph $CODEGRAPH_OUT_BASE/myrepo/graph.json"
}

@test "path forwards both endpoints and --graph" {
    run "$SCRIPT" path "A" "B" "$REPO"
    assert_success
    run cat "$GRAPHIFY_STUB_LOG"
    assert_output --partial "ARGS: path A B --graph $CODEGRAPH_OUT_BASE/myrepo/graph.json"
}

@test "missing graphify binary prints actionable error" {
    rm "$CODEGRAPH_VENV/bin/graphify"
    run "$SCRIPT" "$REPO"
    assert_failure
    assert_output --partial "graphify not found"
    assert_output --partial "scripts/codegraph install"
}

@test "build rejects a non-existent target directory" {
    run "$SCRIPT" "$TEST_DIR/does-not-exist"
    assert_failure
    assert_output --partial "is not a directory"
}

@test "query without a question prints an error" {
    run "$SCRIPT" query
    assert_failure
    assert_output --partial "question required"
}

@test "explain without a symbol prints an error" {
    run "$SCRIPT" explain
    assert_failure
    assert_output --partial "symbol required"
}
