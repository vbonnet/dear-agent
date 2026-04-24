#!/usr/bin/env bats
# Tests for agm/scripts/pre-install.sh

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'

    TEST_DIR="$(mktemp -d)"

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    PRE_INSTALL="$PROJECT_ROOT/agm/scripts/pre-install.sh"
}

teardown() {
    rm -rf "$TEST_DIR"
}

@test "pre-install.sh has set -euo pipefail" {
    run head -6 "$PRE_INSTALL"
    assert_output --partial "set -euo pipefail"
}

@test "pre-install.sh rejects missing workspace directory" {
    run bash "$PRE_INSTALL" "/nonexistent/path" "1.0.0"
    assert_failure
    assert_output --partial "does not exist"
}

@test "pre-install.sh validates existing workspace" {
    run bash "$PRE_INSTALL" "$TEST_DIR" "1.0.0"
    # Should pass workspace check (may fail on other checks depending on environment)
    assert_output --partial "Workspace directory exists"
}

@test "pre-install.sh skips if already initialized" {
    touch "$TEST_DIR/.agm-initialized"
    run bash "$PRE_INSTALL" "$TEST_DIR" "1.0.0"
    assert_success
    assert_output --partial "already initialized"
}
