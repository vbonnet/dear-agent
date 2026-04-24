#!/bin/bash
# test-enforcement-gates.sh — Tests for the test enforcement gate hooks.
# Creates a temporary git repo and verifies hook behavior.

set -euo pipefail

PASS=0
FAIL=0
TESTS=0

# Setup temp directory
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Copy hook files to temp
HOOK_SRC="$(dirname "$0")"

setup_repo() {
    rm -rf "$TMPDIR/repo"
    mkdir -p "$TMPDIR/repo"
    git -C "$TMPDIR/repo" init -q
    git -C "$TMPDIR/repo" config user.email "test@test.com"
    git -C "$TMPDIR/repo" config user.name "Test"
    # Install hooks
    mkdir -p "$TMPDIR/repo/.githooks"
    cp "$HOOK_SRC/test-gate-common.sh" "$TMPDIR/repo/.githooks/"
    cp "$HOOK_SRC/pre-commit" "$TMPDIR/repo/.githooks/"
    chmod +x "$TMPDIR/repo/.githooks/pre-commit" "$TMPDIR/repo/.githooks/test-gate-common.sh"
    git -C "$TMPDIR/repo" config core.hooksPath .githooks
    # Initial commit so HEAD exists
    touch "$TMPDIR/repo/.gitkeep"
    git -C "$TMPDIR/repo" add .gitkeep
    git -C "$TMPDIR/repo" commit -q -m "initial"
}

assert_blocked() {
    local test_name="$1"
    local exit_code="$2"
    TESTS=$((TESTS + 1))
    if [ "$exit_code" -ne 0 ]; then
        echo "  ✅ PASS: $test_name (blocked as expected)"
        PASS=$((PASS + 1))
    else
        echo "  ❌ FAIL: $test_name (should have been blocked but was allowed)"
        FAIL=$((FAIL + 1))
    fi
}

assert_allowed() {
    local test_name="$1"
    local exit_code="$2"
    TESTS=$((TESTS + 1))
    if [ "$exit_code" -eq 0 ]; then
        echo "  ✅ PASS: $test_name (allowed as expected)"
        PASS=$((PASS + 1))
    else
        echo "  ❌ FAIL: $test_name (should have been allowed but was blocked)"
        FAIL=$((FAIL + 1))
    fi
}

assert_output_contains() {
    local test_name="$1"
    local output="$2"
    local pattern="$3"
    TESTS=$((TESTS + 1))
    if echo "$output" | grep -qE "$pattern"; then
        echo "  ✅ PASS: $test_name (output contains expected pattern)"
        PASS=$((PASS + 1))
    else
        echo "  ❌ FAIL: $test_name (output missing pattern: $pattern)"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== Test Enforcement Gates ==="
echo ""

# --- Test 1: Code-only commit (no tests) → blocked ---
echo "Test 1: Code-only commit without tests"
setup_repo
echo 'package main' > "$TMPDIR/repo/main.go"
echo 'func Hello() string { return "hello" }' >> "$TMPDIR/repo/main.go"
git -C "$TMPDIR/repo" add main.go
output=$(git -C "$TMPDIR/repo" commit -m "code only" 2>&1 || true)
exit_code=$?
# Check if the commit was actually created (it shouldn't be)
if git -C "$TMPDIR/repo" log --oneline -1 | grep -q "code only"; then
    assert_blocked "Code-only commit blocked" 0
else
    assert_blocked "Code-only commit blocked" 1
fi
assert_output_contains "Error message present" "$output" "TEST ENFORCEMENT GATE"

# --- Test 2: Code + test commit → allowed ---
echo ""
echo "Test 2: Code + test commit"
setup_repo
echo 'package main' > "$TMPDIR/repo/main.go"
echo 'func Hello() string { return "hello" }' >> "$TMPDIR/repo/main.go"
cat > "$TMPDIR/repo/main_test.go" << 'GOTEST'
package main

import "testing"

func TestHello(t *testing.T) {
    if Hello() != "hello" {
        t.Error("unexpected")
    }
}
GOTEST
git -C "$TMPDIR/repo" add main.go main_test.go
git -C "$TMPDIR/repo" commit -m "code with tests" 2>&1
exit_code=$?
assert_allowed "Code + test commit allowed" "$exit_code"

# --- Test 3: t.Skip in new tests → warning flagged ---
echo ""
echo "Test 3: t.Skip in test files"
setup_repo
echo 'package main' > "$TMPDIR/repo/feature.go"
echo 'func Feature() {}' >> "$TMPDIR/repo/feature.go"
cat > "$TMPDIR/repo/feature_test.go" << 'GOTEST'
package main

import "testing"

func TestFeature(t *testing.T) {
    t.Skip("not implemented yet")
}
GOTEST
git -C "$TMPDIR/repo" add feature.go feature_test.go
output=$(git -C "$TMPDIR/repo" commit -m "skip test" 2>&1)
exit_code=$?
assert_allowed "Commit with t.Skip allowed (warning only)" "$exit_code"
assert_output_contains "t.Skip warning flagged" "$output" "t\.Skip"

# --- Test 4: TODO test in code → warning flagged ---
echo ""
echo "Test 4: TODO test pattern in code"
setup_repo
cat > "$TMPDIR/repo/handler.go" << 'GOCODE'
package main

// TODO add test for error handling
func Handler() error { return nil }
GOCODE
cat > "$TMPDIR/repo/handler_test.go" << 'GOTEST'
package main

import "testing"

func TestHandler(t *testing.T) {
    if err := Handler(); err != nil {
        t.Error(err)
    }
}
GOTEST
git -C "$TMPDIR/repo" add handler.go handler_test.go
output=$(git -C "$TMPDIR/repo" commit -m "todo test" 2>&1)
exit_code=$?
assert_allowed "Commit with TODO test allowed (warning only)" "$exit_code"
assert_output_contains "TODO test warning flagged" "$output" "TODO.*test|Deferred test"

# --- Test 5: AGM_SKIP_TEST_GATE=1 override → allowed ---
echo ""
echo "Test 5: Override with AGM_SKIP_TEST_GATE=1"
setup_repo
echo 'package main' > "$TMPDIR/repo/skip.go"
echo 'func Skip() {}' >> "$TMPDIR/repo/skip.go"
git -C "$TMPDIR/repo" add skip.go
AGM_SKIP_TEST_GATE=1 git -C "$TMPDIR/repo" commit -m "override" 2>&1
exit_code=$?
assert_allowed "Override with AGM_SKIP_TEST_GATE=1" "$exit_code"

# --- Test 6: Infrastructure-only changes → allowed ---
echo ""
echo "Test 6: Infrastructure-only changes (no tests needed)"
setup_repo
echo '# Updated README' > "$TMPDIR/repo/README.md"
echo '.PHONY: all' > "$TMPDIR/repo/Makefile"
git -C "$TMPDIR/repo" add README.md Makefile
git -C "$TMPDIR/repo" commit -m "docs only" 2>&1
exit_code=$?
assert_allowed "Infrastructure-only changes allowed" "$exit_code"

# --- Test 7: Test-only changes → allowed ---
echo ""
echo "Test 7: Test-only changes (no code files)"
setup_repo
# First add some code with tests
echo 'package main' > "$TMPDIR/repo/existing.go"
echo 'func Existing() {}' >> "$TMPDIR/repo/existing.go"
echo 'package main' > "$TMPDIR/repo/existing_test.go"
git -C "$TMPDIR/repo" add existing.go existing_test.go
git -C "$TMPDIR/repo" commit -q -m "base"
# Now modify only test file
cat >> "$TMPDIR/repo/existing_test.go" << 'GOTEST'

import "testing"

func TestExisting(t *testing.T) {}
GOTEST
git -C "$TMPDIR/repo" add existing_test.go
git -C "$TMPDIR/repo" commit -m "test only" 2>&1
exit_code=$?
assert_allowed "Test-only changes allowed" "$exit_code"

# --- Summary ---
echo ""
echo "════════════════════════════════════════"
echo "  Results: $PASS passed, $FAIL failed, $TESTS total"
echo "════════════════════════════════════════"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
