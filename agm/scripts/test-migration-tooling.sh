#!/usr/bin/env bash
#
# Test Suite for AGM Migration Tooling
#
# Purpose: Validate that migration validation script works correctly
# with various session manifest scenarios.
#
# Usage:
#   ./scripts/test-migration-tooling.sh

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test configuration
TEST_DIR="/tmp/agm-migration-test-$$"
VALIDATION_SCRIPT="$(dirname "$0")/agm-migration-validate.sh"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
info() {
    echo -e "${BLUE}ℹ${NC} $*"
}

success() {
    echo -e "${GREEN}✓${NC} $*"
}

error() {
    echo -e "${RED}✗${NC} $*"
}

# Test framework functions
setup_test_env() {
    info "Setting up test environment: $TEST_DIR"
    mkdir -p "$TEST_DIR"
    export SESSIONS_DIR="$TEST_DIR"
}

cleanup_test_env() {
    info "Cleaning up test environment"
    rm -rf "$TEST_DIR"
}

create_test_session() {
    local session_name="$1"
    local manifest_content="$2"

    mkdir -p "$TEST_DIR/$session_name"
    echo "$manifest_content" > "$TEST_DIR/$session_name/manifest.yaml"
}

run_test() {
    local test_name="$1"
    local expected_result="$2"  # "pass" or "fail"
    local session_name="$3"

    ((TESTS_RUN++))

    info "Running test: $test_name"

    if SESSIONS_DIR="$TEST_DIR" "$VALIDATION_SCRIPT" "$session_name" >/dev/null 2>&1; then
        actual_result="pass"
    else
        actual_result="fail"
    fi

    if [[ "$actual_result" == "$expected_result" ]]; then
        success "Test passed: $test_name"
        ((TESTS_PASSED++))
    else
        error "Test failed: $test_name (expected $expected_result, got $actual_result)"
        ((TESTS_FAILED++))
    fi
}

# Test cases

test_valid_manifest_v2() {
    local manifest='schema_version: "2.0"
session_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
name: "test-session"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-02-04T12:00:00Z"
lifecycle: ""
context:
  project: "$HOME/src/test"
  purpose: "Testing"
  tags: []
  notes: ""
claude:
  uuid: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
tmux:
  session_name: "test-session"'

    create_test_session "valid-v2" "$manifest"
    run_test "Valid v2.0 manifest" "pass" "valid-v2"
}

test_valid_manifest_v2_with_agent() {
    local manifest='schema_version: "2.0"
session_id: "b2c3d4e5-f6g7-8901-bcde-fg2345678901"
name: "test-gemini"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-02-04T12:00:00Z"
lifecycle: ""
agent: "gemini"
context:
  project: "$HOME/src/test"
  purpose: "Testing"
  tags: []
  notes: ""
claude:
  uuid: ""
tmux:
  session_name: "test-gemini"'

    create_test_session "valid-v2-agent" "$manifest"
    run_test "Valid v2.0 manifest with agent field" "pass" "valid-v2-agent"
}

test_invalid_yaml_syntax() {
    local manifest='schema_version: "2.0"
session_id: "c3d4e5f6-g7h8-9012-cdef-gh3456789012"
name: test:with:colons:unquoted
created_at: "2026-01-01T00:00:00Z"'

    create_test_session "invalid-yaml" "$manifest"
    run_test "Invalid YAML syntax (unquoted special chars)" "fail" "invalid-yaml"
}

test_missing_required_fields() {
    local manifest='schema_version: "2.0"
name: "incomplete-session"'

    create_test_session "missing-fields" "$manifest"
    run_test "Missing required fields (session_id, created_at)" "fail" "missing-fields"
}

test_invalid_uuid_format() {
    local manifest='schema_version: "2.0"
session_id: "session-old-legacy-format"
name: "legacy-session"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-02-04T12:00:00Z"
lifecycle: ""
context:
  project: "$HOME/src/test"
claude:
  uuid: ""
tmux:
  session_name: "legacy-session"'

    create_test_session "invalid-uuid" "$manifest"
    run_test "Invalid UUID format (legacy session-name format)" "fail" "invalid-uuid"
}

test_unsupported_schema_version() {
    local manifest='schema_version: "1.0"
session_id: "d4e5f6g7-h8i9-0123-defg-hi4567890123"
name: "old-v1-session"
created_at: "2026-01-01T00:00:00Z"'

    create_test_session "unsupported-schema" "$manifest"
    run_test "Unsupported schema version (v1.0)" "fail" "unsupported-schema"
}

test_manifest_not_found() {
    mkdir -p "$TEST_DIR/no-manifest"
    # Don't create manifest.yaml

    run_test "Manifest file not found" "fail" "no-manifest"
}

test_empty_manifest() {
    create_test_session "empty-manifest" ""
    run_test "Empty manifest file" "fail" "empty-manifest"
}

test_corrupt_manifest() {
    local manifest='This is not valid YAML
at all: just random text
no structure: {{{]]]'

    create_test_session "corrupt-manifest" "$manifest"
    run_test "Corrupt manifest (invalid YAML structure)" "fail" "corrupt-manifest"
}

test_valid_uuid_uppercase() {
    local manifest='schema_version: "2.0"
session_id: "E5F6G7H8-I9J0-1234-EFGH-IJ5678901234"
name: "uppercase-uuid"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-02-04T12:00:00Z"
lifecycle: ""
context:
  project: "$HOME/src/test"
claude:
  uuid: ""
tmux:
  session_name: "uppercase-uuid"'

    create_test_session "uppercase-uuid" "$manifest"
    run_test "Uppercase UUID (should fail, UUIDs must be lowercase)" "fail" "uppercase-uuid"
}

test_agent_field_invalid_value() {
    local manifest='schema_version: "2.0"
session_id: "f6g7h8i9-j0k1-2345-fghi-jk6789012345"
name: "invalid-agent"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-02-04T12:00:00Z"
lifecycle: ""
agent: "unknown-agent"
context:
  project: "$HOME/src/test"
claude:
  uuid: ""
tmux:
  session_name: "invalid-agent"'

    create_test_session "invalid-agent" "$manifest"
    run_test "Invalid agent value (not claude/gemini/gpt)" "fail" "invalid-agent"
}

test_schema_version_3_valid() {
    local manifest='schema_version: "3.0"
session_id: "g7h8i9j0-k1l2-3456-ghij-kl7890123456"
name: "v3-session"
created_at: "2026-01-01T00:00:00Z"
updated_at: "2026-02-04T12:00:00Z"
lifecycle: ""
agent: "claude"
context:
  project: "$HOME/src/test"
claude:
  uuid: "g7h8i9j0-k1l2-3456-ghij-kl7890123456"
tmux:
  session_name: "v3-session"'

    create_test_session "v3-valid" "$manifest"
    run_test "Valid v3.0 manifest (future schema)" "pass" "v3-valid"
}

# Main test execution
main() {
    echo "========================================="
    echo "AGM Migration Tooling Test Suite"
    echo "========================================="
    echo

    # Check if validation script exists
    if [[ ! -f "$VALIDATION_SCRIPT" ]]; then
        error "Validation script not found: $VALIDATION_SCRIPT"
        exit 1
    fi

    # Check if validation script is executable
    if [[ ! -x "$VALIDATION_SCRIPT" ]]; then
        info "Making validation script executable"
        chmod +x "$VALIDATION_SCRIPT"
    fi

    setup_test_env

    info "Running test cases..."
    echo

    # Run all tests
    test_valid_manifest_v2
    test_valid_manifest_v2_with_agent
    test_invalid_yaml_syntax
    test_missing_required_fields
    test_invalid_uuid_format
    test_unsupported_schema_version
    test_manifest_not_found
    test_empty_manifest
    test_corrupt_manifest
    test_valid_uuid_uppercase
    test_agent_field_invalid_value
    test_schema_version_3_valid

    cleanup_test_env

    # Print summary
    echo
    echo "========================================="
    echo "Test Summary"
    echo "========================================="
    echo "Tests run:    $TESTS_RUN"
    echo -e "${GREEN}Tests passed:${NC} $TESTS_PASSED"
    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo -e "${RED}Tests failed:${NC} $TESTS_FAILED"
    else
        echo -e "Tests failed: $TESTS_FAILED"
    fi

    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo
        success "All tests passed!"
        exit 0
    else
        echo
        error "Some tests failed"
        exit 1
    fi
}

# Run tests
main
