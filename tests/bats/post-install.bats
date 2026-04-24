#!/usr/bin/env bats
# Tests for agm/scripts/post-install.sh

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    TEST_DIR="$(mktemp -d)"
    export HOME="$TEST_DIR"
    WORKSPACE="$TEST_DIR/workspace"
    mkdir -p "$WORKSPACE"

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    POST_INSTALL="$PROJECT_ROOT/agm/scripts/post-install.sh"
}

teardown() {
    rm -rf "$TEST_DIR"
}

@test "post-install.sh has set -euo pipefail" {
    run head -6 "$POST_INSTALL"
    assert_output --partial "set -euo pipefail"
}

@test "post-install.sh creates session directory" {
    run bash "$POST_INSTALL" "$WORKSPACE"
    assert_dir_exists "$HOME/.agm/sessions"
}

@test "post-install.sh creates cache directory" {
    run bash "$POST_INSTALL" "$WORKSPACE"
    assert_dir_exists "$HOME/.agm-cache"
}

@test "post-install.sh creates config file" {
    run bash "$POST_INSTALL" "$WORKSPACE"
    assert_file_exists "$HOME/.agm/config.yaml"
}

@test "post-install.sh marks workspace as initialized" {
    run bash "$POST_INSTALL" "$WORKSPACE"
    assert_file_exists "$WORKSPACE/.agm-initialized"
}

@test "post-install.sh is idempotent" {
    bash "$POST_INSTALL" "$WORKSPACE"
    run bash "$POST_INSTALL" "$WORKSPACE"
    assert_success
    assert_output --partial "already exists"
}
