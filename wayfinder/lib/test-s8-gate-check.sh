#!/bin/bash
# test-s8-gate-check.sh: Unit tests for s8-gate-check.sh
# Tests all scenarios from gate-3-violation-fix.md Phase 2

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
S8_GATE_CHECK="$SCRIPT_DIR/s8-gate-check.sh"

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
    temp_dir=$(mktemp -d -t "s8-test-$name-XXXXXX")
    TEMP_DIRS+=("$temp_dir")
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
    "$S8_GATE_CHECK" "$project_dir" > /dev/null 2>&1
    local actual_exit=$?
    set -e

    assert_exit_code "$expected_exit" "$actual_exit" "$test_name"
}

# ============================================================================
# Test 1: Normal code project (no test bead) → should pass
# ============================================================================

test_normal_code_project() {
    local project_dir
    project_dir=$(create_test_project "normal-code")

    # Create W0 charter (not a test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Feature implementation
Goal: Add new API endpoint
EOF

    # Create S8 deliverable
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Implemented new API endpoint in main.go
EOF

    # Create actual code files
    mkdir -p "$project_dir/src"
    cat > "$project_dir/src/main.go" <<EOF
package main

func main() {
    println("Hello, World!")
}
EOF

    run_test "Test 1: Normal code project (no test bead)" 0 "$project_dir"
}

# ============================================================================
# Test 2: Test bead with actual test files → should pass
# ============================================================================

test_test_bead_with_tests() {
    local project_dir
    project_dir=$(create_test_project "test-bead-with-tests")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Implement test suite for authentication module
EOF

    # Create S8 deliverable
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Created 15 tests in auth_test.go
EOF

    # Create actual test files
    mkdir -p "$project_dir/tests"
    cat > "$project_dir/tests/auth_test.go" <<EOF
package tests

import "testing"

func TestAuthentication(t *testing.T) {
    // Test implementation
}
EOF

    run_test "Test 2: Test bead with actual test files" 0 "$project_dir"
}

# ============================================================================
# Test 3: Test bead with zero test files → should fail
# ============================================================================

test_test_bead_no_tests() {
    local project_dir
    project_dir=$(create_test_project "test-bead-no-tests")

    # Create W0 charter (test bead)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Testing
Goal: Add tests for payment module
EOF

    # Create S8 deliverable (but no actual test files)
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Tests would be implemented in payment_test.go
EOF

    # Create regular code file but no test files
    mkdir -p "$project_dir/src"
    cat > "$project_dir/src/payment.go" <<EOF
package payment

func ProcessPayment() bool {
    return true
}
EOF

    run_test "Test 3: Test bead with zero test files" 1 "$project_dir"
}

# ============================================================================
# Test 4: S8 document with "would implement" → should fail
# ============================================================================

test_red_flag_would_implement() {
    local project_dir
    project_dir=$(create_test_project "red-flag-would-implement")

    # Create W0 charter
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Feature
Goal: Implement user authentication
EOF

    # Create S8 with red flag pattern
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

## What Would Be Implemented

The authentication system would implement the following features:
- Login endpoint
- Token generation
- Session management
EOF

    # Create code files (to pass code file check)
    mkdir -p "$project_dir/src"
    cat > "$project_dir/src/auth.go" <<EOF
package auth
EOF

    run_test "Test 4: Red flag pattern 'would implement'" 1 "$project_dir"
}

# ============================================================================
# Test 5: S8 document with "demonstration" → should fail
# ============================================================================

test_red_flag_demonstration() {
    local project_dir
    project_dir=$(create_test_project "red-flag-demonstration")

    # Create W0 charter
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Feature
Goal: Implement caching layer
EOF

    # Create S8 with red flag pattern
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

This is a Wayfinder demonstration of how caching would be implemented.

## Cache Strategy

The demonstration shows a Redis-based cache implementation.
EOF

    # Create code files
    mkdir -p "$project_dir/src"
    cat > "$project_dir/src/cache.go" <<EOF
package cache
EOF

    run_test "Test 5: Red flag pattern 'demonstration'" 1 "$project_dir"
}

# ============================================================================
# Test 6: S8 document with "conceptual" → should fail
# ============================================================================

test_red_flag_conceptual() {
    local project_dir
    project_dir=$(create_test_project "red-flag-conceptual")

    # Create W0 charter
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Feature
Goal: Database migration system
EOF

    # Create S8 with red flag pattern
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

## Migration System (Conceptual)

✓ (Conceptual) Database schema migrations
✓ (Conceptual) Rollback mechanism
✓ (Conceptual) Version tracking
EOF

    # Create code files
    mkdir -p "$project_dir/src"
    cat > "$project_dir/src/migrate.go" <<EOF
package migrate
EOF

    run_test "Test 6: Red flag pattern 'conceptual'" 1 "$project_dir"
}

# ============================================================================
# Test 7: S8 document with multiple red flags → should fail
# ============================================================================

test_multiple_red_flags() {
    local project_dir
    project_dir=$(create_test_project "multiple-red-flags")

    # Create W0 charter
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Feature
Goal: API gateway
EOF

    # Create S8 with multiple red flag patterns
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

This is a demonstration of what would be implemented.

## Conceptual Design

The blueprint shows how the API gateway would be structured.
EOF

    # Create code files
    mkdir -p "$project_dir/src"
    cat > "$project_dir/src/gateway.go" <<EOF
package gateway
EOF

    run_test "Test 7: Multiple red flag patterns" 1 "$project_dir"
}

# ============================================================================
# Test 8: No code files (non-doc-only project) → should fail
# ============================================================================

test_no_code_files() {
    local project_dir
    project_dir=$(create_test_project "no-code-files")

    # Create W0 charter
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Feature
Goal: Implement logging
EOF

    # Create S8 deliverable but no code files
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Logging implementation completed.
EOF

    run_test "Test 8: No code files created" 1 "$project_dir"
}

# ============================================================================
# Test 9: Documentation-only project → should pass
# ============================================================================

test_documentation_only() {
    local project_dir
    project_dir=$(create_test_project "doc-only")

    # Create W0 charter (documentation-only)
    cat > "$project_dir/W0-charter.md" <<EOF
# Project Charter

Type: Documentation only
Goal: Update README and API docs
EOF

    # Create S8 deliverable
    cat > "$project_dir/S8-implementation.md" <<EOF
# S8 Implementation

Updated README.md with installation instructions.
Updated API documentation.
EOF

    # Create markdown files (no code files needed for doc-only)
    cat > "$project_dir/README.md" <<EOF
# API Documentation

Installation instructions...
EOF

    run_test "Test 9: Documentation-only project" 0 "$project_dir"
}

# ============================================================================
# Test 10: S8 deliverable doesn't exist yet → should pass silently
# ============================================================================

test_no_s8_deliverable() {
    local project_dir
    project_dir=$(create_test_project "no-s8")

    # Don't create S8-implementation.md

    run_test "Test 10: S8 deliverable doesn't exist yet" 0 "$project_dir"
}

# ============================================================================
# Run all tests
# ============================================================================

echo "========================================"
echo "S8 Gate Check Unit Tests"
echo "========================================"

test_normal_code_project
test_test_bead_with_tests
test_test_bead_no_tests
test_red_flag_would_implement
test_red_flag_demonstration
test_red_flag_conceptual
test_multiple_red_flags
test_no_code_files
test_documentation_only
test_no_s8_deliverable

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
