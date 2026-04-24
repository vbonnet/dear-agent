#!/bin/bash
# test-s9-test-count-verification.sh: Unit tests for s9-test-count-verification.sh
# Tests all scenarios from gate-3-violation-fix.md Phase 2

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
S9_TEST_COUNT_VERIFICATION="$SCRIPT_DIR/s9-test-count-verification.sh"

# Test utilities
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
TEMP_DIRS=()

cleanup() {
    for dir in "${TEMP_DIRS[@]}"; do
        rm -rf "$dir"
    done
}
trap cleanup EXIT

create_test_project() {
    local name="$1"
    local temp_dir
    temp_dir=$(mktemp -d -t "s9-test-$name-XXXXXX")
    TEMP_DIRS+=("$temp_dir")

    # Initialize as git repo (needed for test file verification)
    (cd "$temp_dir" && git init -q && git config user.email "test@example.com" && git config user.name "Test User")

    echo "$temp_dir"
}

assert_exit_code() {
    local expected="$1"
    local actual="$2"
    local test_name="$3"

    TESTS_RUN=$((TESTS_RUN + 1))

    if [[ "$expected" -eq "$actual" ]]; then
        echo "✅ PASS: $test_name (exit code: $actual)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo "❌ FAIL: $test_name (expected exit code: $expected, got: $actual)"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

run_test() {
    local test_name="$1"
    local expected_exit="$2"
    local project_dir="$3"

    echo ""
    echo "Running: $test_name"
    echo "Project: $project_dir"

    set +e
    "$S9_TEST_COUNT_VERIFICATION" "$project_dir" > /dev/null 2>&1
    local actual_exit=$?
    set -e

    assert_exit_code "$expected_exit" "$actual_exit" "$test_name"
}

# ============================================================================
# Test 1: Not a test bead → should skip gracefully
# ============================================================================

test_not_test_bead() {
    local project_dir
    project_dir=$(create_test_project "not-test-bead")

    # Create W0 charter (not a test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Feature implementation
Goal: Add new API endpoint
EOF

    run_test "Test 1: Not a test bead (should skip)" 0 "$project_dir"
}

# ============================================================================
# Test 2: Test bead with matching claims (Jest/npm) → should pass
# ============================================================================

test_matching_claims_jest() {
    local project_dir
    project_dir=$(create_test_project "matching-claims-jest")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Implement test suite for authentication
EOF

    # Create S8 deliverable claiming 5 tests
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Created 5 new tests for authentication module.
Total: 5 tests passing
EOF

    # Create S9 deliverable
    cat > "$project_dir/S9-validation.md" <<EOF
# S9 Validation

Tests: 5 passed
Coverage: 80%
EOF

    # Create package.json and actual tests
    cat > "$project_dir/package.json" <<EOF
{
  "name": "test-project",
  "scripts": {
    "test": "echo 'Tests: 5 passed, 5 total' && exit 0"
  }
}
EOF

    mkdir -p "$project_dir/tests"
    cat > "$project_dir/tests/auth.test.ts" <<EOF
import { describe, it, expect } from '@jest/globals';

describe('Auth', () => {
  it('test 1', () => expect(true).toBe(true));
  it('test 2', () => expect(true).toBe(true));
  it('test 3', () => expect(true).toBe(true));
  it('test 4', () => expect(true).toBe(true));
  it('test 5', () => expect(true).toBe(true));
});
EOF

    # Commit test files
    (cd "$project_dir" && git add . && git commit -q -m "Add tests")

    run_test "Test 2: Test bead with matching claims (Jest)" 0 "$project_dir"
}

# ============================================================================
# Test 3: Claimed 260 tests, actual 214 → should fail (oss-n1nq.12 pattern)
# ============================================================================

test_fabricated_metrics_oss_n1nq12() {
    local project_dir
    project_dir=$(create_test_project "fabricated-oss-n1nq12")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Auth module test suite
EOF

    # Create S8 deliverable claiming 260 tests (fabricated)
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Created 45 new tests in 4 files.
Total: 260 tests passing
Coverage: 90.54%
EOF

    # Create S9 deliverable (fabricated validation)
    cat > "$project_dir/S9-validation.md" <<EOF
# S9 Validation

Tests: 260 passed
214 baseline + 46 new tests
EOF

    # Create package.json that returns 214 tests (actual baseline)
    cat > "$project_dir/package.json" <<EOF
{
  "name": "test-project",
  "scripts": {
    "test": "echo 'Tests: 214 passed, 214 total' && exit 0"
  }
}
EOF

    # No new test files in git history (simulating oss-n1nq.12)
    mkdir -p "$project_dir/tests"
    cat > "$project_dir/tests/existing.test.ts" <<EOF
// Existing tests (baseline)
EOF
    (cd "$project_dir" && git add . && git commit -q -m "Initial commit (baseline)")

    run_test "Test 3: Fabricated metrics (oss-n1nq.12 pattern)" 1 "$project_dir"
}

# ============================================================================
# Test 4: Claims new tests, no git history → should fail
# ============================================================================

test_claimed_tests_no_git_history() {
    local project_dir
    project_dir=$(create_test_project "no-git-history")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Add unit tests
EOF

    # Create S8 claiming new tests
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Created 10 new tests in test_auth.py
EOF

    # Create S9
    cat > "$project_dir/S9-validation.md" <<EOF
# S9 Validation

+10 new tests
EOF

    # Create test files but don't commit them as "new" (simulate claiming tests that weren't added)
    mkdir -p "$project_dir/tests"
    cat > "$project_dir/tests/existing_test.py" <<EOF
# Old tests
EOF
    (cd "$project_dir" && git add . && git commit -q -m "Baseline")

    # Fake package.json that passes tests but no new test files in history
    cat > "$project_dir/package.json" <<EOF
{
  "name": "test-project",
  "scripts": {
    "test": "echo '10 passing' && exit 0"
  }
}
EOF

    run_test "Test 4: Claims new tests but no git history" 1 "$project_dir"
}

# ============================================================================
# Test 5: Go project with matching claims → should pass
# ============================================================================

test_matching_claims_go() {
    local project_dir
    project_dir=$(create_test_project "matching-claims-go")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Add tests for auth module
EOF

    # Create S8 claiming 3 tests
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Created 3 tests in auth_test.go
EOF

    # Create go.mod to simulate Go project
    cat > "$project_dir/go.mod" <<EOF
module example.com/test

go 1.21
EOF

    # Create test file
    cat > "$project_dir/auth_test.go" <<EOF
package main

import "testing"

func TestAuth1(t *testing.T) {}
func TestAuth2(t *testing.T) {}
func TestAuth3(t *testing.T) {}
EOF

    # Commit test files
    (cd "$project_dir" && git add . && git commit -q -m "Add tests")

    # Mock 'go test' to succeed with 3 passing tests
    # Note: This test will only pass if 'go test' is available
    # For full test coverage, we'd need to mock the go binary

    run_test "Test 5: Go project with matching claims" 0 "$project_dir"
}

# ============================================================================
# Test 6: Test framework not detected → should skip gracefully
# ============================================================================

test_unknown_test_framework() {
    local project_dir
    project_dir=$(create_test_project "unknown-framework")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Add tests
EOF

    # Create S8/S9 deliverables
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Tests created
EOF

    cat > "$project_dir/S9-validation.md" <<EOF
# S9 Validation

Tests: 5 passed
EOF

    # Don't create package.json, go.mod, or Python tests
    # Script should skip gracefully

    run_test "Test 6: Unknown test framework (should skip)" 0 "$project_dir"
}

# ============================================================================
# Test 7: Tests fail to execute → should fail
# ============================================================================

test_tests_fail_execution() {
    local project_dir
    project_dir=$(create_test_project "failing-tests")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Test suite
EOF

    # Create package.json with failing test script
    cat > "$project_dir/package.json" <<EOF
{
  "name": "test-project",
  "scripts": {
    "test": "exit 1"
  }
}
EOF

    # Create S8/S9 deliverables claiming tests pass
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

All tests pass
EOF

    run_test "Test 7: Tests fail to execute" 1 "$project_dir"
}

# ============================================================================
# Test 8: S8/S9 deliverables don't exist → should skip gracefully
# ============================================================================

test_no_deliverables() {
    local project_dir
    project_dir=$(create_test_project "no-deliverables")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Tests
EOF

    # Don't create S8/S9 deliverables
    # Should skip gracefully (nothing to verify)

    run_test "Test 8: No S8/S9 deliverables (should skip)" 0 "$project_dir"
}

# ============================================================================
# Run all tests
# ============================================================================

echo "========================================"
echo "S9 Test Count Verification Unit Tests"
echo "========================================"

test_not_test_bead
test_matching_claims_jest
test_fabricated_metrics_oss_n1nq12
test_claimed_tests_no_git_history
test_matching_claims_go
test_unknown_test_framework
test_tests_fail_execution
test_no_deliverables

# ============================================================================
# Report results
# ============================================================================

echo ""
echo "========================================"
echo "Test Results"
echo "========================================"
echo "Tests run:    $TESTS_RUN"
echo "Tests passed: $TESTS_PASSED"
echo "Tests failed: $TESTS_FAILED"
echo "========================================"

if [[ "$TESTS_FAILED" -eq 0 ]]; then
    echo "✅ All tests passed!"
    exit 0
else
    echo "❌ Some tests failed"
    exit 1
fi
