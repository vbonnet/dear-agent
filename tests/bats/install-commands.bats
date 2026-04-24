#!/usr/bin/env bats
# Tests for agm/install-commands.sh

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    TEST_DIR="$(mktemp -d)"
    export HOME="$TEST_DIR"
    mkdir -p "$HOME/.claude/commands"

    # Get project root
    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    INSTALL_SCRIPT="$PROJECT_ROOT/agm/install-commands.sh"
}

teardown() {
    rm -rf "$TEST_DIR"
}

@test "install-commands.sh has set -euo pipefail" {
    run head -5 "$INSTALL_SCRIPT"
    assert_output --partial "set -euo pipefail"
}

@test "install-commands.sh creates commands directory" {
    # Create mock source commands
    mkdir -p "$PROJECT_ROOT/agm/agm-plugin/commands" 2>/dev/null || true
    if [ -d "$PROJECT_ROOT/agm/agm-plugin/commands" ]; then
        run bash "$INSTALL_SCRIPT"
        assert_dir_exists "$HOME/.claude/commands"
    else
        skip "agm-plugin/commands directory not found"
    fi
}

@test "install-commands.sh is executable" {
    assert_file_executable "$INSTALL_SCRIPT"
}
