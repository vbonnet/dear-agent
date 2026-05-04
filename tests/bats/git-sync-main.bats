#!/usr/bin/env bats
# Tests for scripts/git-sync-main.sh — sanctioned stash/rebase/pop wrapper.
#
# Each test stands up a tiny bare "origin", a feature clone, and exercises
# one path through the script. The fixtures are torn down per-test so
# stash state never leaks between cases.

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    TEST_DIR="$(mktemp -d)"
    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    SCRIPT="$PROJECT_ROOT/scripts/git-sync-main.sh"

    ORIGIN="$TEST_DIR/origin.git"
    WORK="$TEST_DIR/work"
    SEED="$TEST_DIR/seed"

    # Bare origin with a main branch.
    git init -q --bare "$ORIGIN"
    git -C "$ORIGIN" symbolic-ref HEAD refs/heads/main

    # Seed clone publishes the initial commit on main.
    git clone -q "$ORIGIN" "$SEED"
    git -C "$SEED" config user.email "test@example.com"
    git -C "$SEED" config user.name "Test"
    echo "v1" > "$SEED/a.txt"
    git -C "$SEED" add a.txt
    git -C "$SEED" commit -q -m "init"
    git -C "$SEED" branch -M main
    git -C "$SEED" push -q -u origin main

    # Work clone — the script's target.
    git clone -q "$ORIGIN" "$WORK"
    git -C "$WORK" config user.email "test@example.com"
    git -C "$WORK" config user.name "Test"
}

teardown() {
    rm -rf "$TEST_DIR"
}

# Helper: push a new commit to origin/main from the seed clone.
push_to_main() {
    local file=$1 content=$2 msg=$3
    echo "$content" > "$SEED/$file"
    git -C "$SEED" add "$file"
    git -C "$SEED" commit -q -m "$msg"
    git -C "$SEED" push -q origin main
}

@test "git-sync-main.sh is executable" {
    assert_file_executable "$SCRIPT"
}

@test "git-sync-main.sh uses set -euo pipefail" {
    run head -40 "$SCRIPT"
    assert_output --partial "set -euo pipefail"
}

@test "missing argument prints usage and exits 1" {
    run "$SCRIPT"
    assert_failure 1
    assert_output --partial "Usage:"
}

@test "non-existent path exits 1" {
    run "$SCRIPT" "$TEST_DIR/does-not-exist"
    assert_failure 1
    assert_output --partial "is not a directory"
}

@test "non-git directory exits 1" {
    mkdir -p "$TEST_DIR/plain"
    run "$SCRIPT" "$TEST_DIR/plain"
    assert_failure 1
    assert_output --partial "is not a git repository"
}

@test "detached HEAD is rejected" {
    sha=$(git -C "$WORK" rev-parse HEAD)
    git -C "$WORK" checkout -q --detach "$sha"
    run "$SCRIPT" "$WORK"
    assert_failure 1
    assert_output --partial "detached HEAD"
}

@test "clean tree, already up to date: succeeds and is idempotent" {
    run "$SCRIPT" "$WORK"
    assert_success
    assert_output --partial "already up to date"
    assert_output --partial "done"

    # Second invocation must also succeed cleanly.
    run "$SCRIPT" "$WORK"
    assert_success
    assert_output --partial "done"
}

@test "clean tree, branch behind: rebases without stashing" {
    git -C "$WORK" checkout -q -b feature
    push_to_main "b.txt" "v1" "add b"

    run "$SCRIPT" "$WORK"
    assert_success
    assert_output --partial "no stash needed"
    assert_output --partial "rebased:"
    assert_file_exists "$WORK/b.txt"

    # No leftover stash entries.
    run git -C "$WORK" stash list
    assert_output ""
}

@test "dirty tree, branch behind: stashes, rebases, pops cleanly" {
    git -C "$WORK" checkout -q -b feature
    push_to_main "b.txt" "v1" "add b"

    # Modify a tracked file and add an untracked file in the work clone.
    echo "modified" > "$WORK/a.txt"
    echo "untracked" > "$WORK/untracked.txt"

    run "$SCRIPT" "$WORK"
    assert_success
    assert_output --partial "stashing tracked local changes"
    assert_output --partial "rebased:"
    assert_output --partial "popping stash"

    # Modification preserved across rebase.
    run cat "$WORK/a.txt"
    assert_output "modified"
    assert_file_exists "$WORK/b.txt"
    assert_file_exists "$WORK/untracked.txt"

    # Stash was popped — none should remain.
    run git -C "$WORK" stash list
    assert_output ""
}

@test "rebase conflict: aborts, restores stash, exits 2" {
    git -C "$WORK" checkout -q -b feature
    echo "v-feature" > "$WORK/a.txt"
    git -C "$WORK" commit -q -am "feature change"

    # origin/main also touches a.txt — guaranteed conflict on rebase.
    push_to_main "a.txt" "v-main" "main change"

    # Add a dirty change so the stash path is exercised too.
    echo "dirty" > "$WORK/a.txt"
    head_before=$(git -C "$WORK" rev-parse HEAD)

    run "$SCRIPT" "$WORK"
    assert_failure 2
    assert_output --partial "rebase failed"

    # Branch HEAD restored to its pre-rebase state.
    run git -C "$WORK" rev-parse HEAD
    assert_output "$head_before"

    # Working tree restored.
    run cat "$WORK/a.txt"
    assert_output "dirty"

    # No orphan stash left behind.
    run git -C "$WORK" stash list
    assert_output ""

    # No rebase in progress.
    [ ! -d "$WORK/.git/rebase-merge" ]
    [ ! -d "$WORK/.git/rebase-apply" ]
}

@test "falls back to origin/main when origin/HEAD symbolic ref is missing" {
    # Some clones don't have refs/remotes/origin/HEAD set. Remove it and
    # confirm the script still resolves the target.
    rm -f "$WORK/.git/refs/remotes/origin/HEAD"
    git -C "$WORK" checkout -q -b feature
    push_to_main "b.txt" "v1" "add b"

    run "$SCRIPT" "$WORK"
    assert_success
    assert_output --partial "target=origin/main"
    assert_file_exists "$WORK/b.txt"
}
