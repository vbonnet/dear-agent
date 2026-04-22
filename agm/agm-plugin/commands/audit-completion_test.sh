#!/usr/bin/env bash
# Test suite for audit-completion skill detection logic
# Tests the git analysis checks that the skill performs
# Does NOT test the Claude skill execution itself — tests the underlying detection logic

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR=""
FAILED=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
log_test() {
    echo -e "${YELLOW}TEST:${NC} $1"
}

log_pass() {
    echo -e "${GREEN}PASS:${NC} $1"
}

log_fail() {
    echo -e "${RED}FAIL:${NC} $1"
    FAILED=$((FAILED + 1))
}

cleanup() {
    if [ -n "$TEST_DIR" ] && [ -d "$TEST_DIR" ]; then
        rm -rf "$TEST_DIR"
    fi
}

trap cleanup EXIT

setup_test_repo() {
    TEST_DIR="$(mktemp -d)"
    git -C "$TEST_DIR" init -b main
    git -C "$TEST_DIR" config user.email "test@test.com"
    git -C "$TEST_DIR" config user.name "Test"

    # Create initial commit on main
    echo "package main" > "$TEST_DIR/main.go"
    echo "# Project" > "$TEST_DIR/README.md"
    mkdir -p "$TEST_DIR/pkg/core"
    echo "package core" > "$TEST_DIR/pkg/core/core.go"
    echo "package core" > "$TEST_DIR/pkg/core/core_test.go"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Initial commit"
}

# -------------------------------------------------------------------
# Test 1: Detect docs-only changes
# -------------------------------------------------------------------
test_docs_only_changes() {
    log_test "Detect docs-only changes"
    setup_test_repo

    git -C "$TEST_DIR" checkout -b docs-only-branch
    echo "## Updated" >> "$TEST_DIR/README.md"
    echo "Some notes" > "$TEST_DIR/CHANGELOG.md"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Update documentation"

    # Check: all changed files should be docs
    changed=$(git -C "$TEST_DIR" diff --name-only main..docs-only-branch)
    has_code=false
    while IFS= read -r file; do
        case "$file" in
            *.md|README*|LICENSE*|CHANGELOG*|*.txt|.gitignore) ;;
            *) has_code=true ;;
        esac
    done <<< "$changed"

    if [ "$has_code" = "false" ]; then
        log_pass "Correctly detected docs-only changes"
    else
        log_fail "Should have detected docs-only changes"
    fi

    cleanup
    TEST_DIR=""
}

# -------------------------------------------------------------------
# Test 2: Detect missing test files
# -------------------------------------------------------------------
test_missing_tests() {
    log_test "Detect missing test files for new Go packages"
    setup_test_repo

    git -C "$TEST_DIR" checkout -b missing-tests-branch
    mkdir -p "$TEST_DIR/pkg/newpkg"
    echo "package newpkg" > "$TEST_DIR/pkg/newpkg/handler.go"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Add new package without tests"

    # Check: find Go packages with changes but no _test.go files
    changed=$(git -C "$TEST_DIR" diff --name-only main..missing-tests-branch)
    missing_tests=false
    while IFS= read -r file; do
        case "$file" in
            *_test.go) ;;
            *.go)
                dir="$TEST_DIR/$(dirname "$file")"
                if ! ls "$dir"/*_test.go >/dev/null 2>&1; then
                    missing_tests=true
                fi
                ;;
        esac
    done <<< "$changed"

    if [ "$missing_tests" = "true" ]; then
        log_pass "Correctly detected missing tests in pkg/newpkg"
    else
        log_fail "Should have detected missing tests"
    fi

    cleanup
    TEST_DIR=""
}

# -------------------------------------------------------------------
# Test 3: Detect deferred items in commit messages
# -------------------------------------------------------------------
test_deferred_markers() {
    log_test "Detect deferred/TODO markers in commit messages"
    setup_test_repo

    git -C "$TEST_DIR" checkout -b deferred-branch
    echo "package main // v2" > "$TEST_DIR/main.go"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Refactor main - TODO: add error handling"

    echo "package main // v3" > "$TEST_DIR/main.go"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Update logic

deferred: integration tests will follow in next session"

    # Check: search commit messages for deferred markers
    messages=$(git -C "$TEST_DIR" log --format="%s%n%b" main..deferred-branch)
    found_markers=""

    for pattern in "TODO" "FIXME" "HACK" "XXX" "deferred" "follow-up" "follow up" "punt" "skip for now" "will address later" "out of scope"; do
        if echo "$messages" | grep -qi "$pattern"; then
            found_markers="${found_markers}${pattern}, "
        fi
    done

    if [ -n "$found_markers" ]; then
        log_pass "Correctly detected deferred markers: ${found_markers%, }"
    else
        log_fail "Should have detected TODO and deferred markers"
    fi

    cleanup
    TEST_DIR=""
}

# -------------------------------------------------------------------
# Test 4: Detect code changes (not docs-only)
# -------------------------------------------------------------------
test_code_changes_detected() {
    log_test "Detect that branch has real code changes (not docs-only)"
    setup_test_repo

    git -C "$TEST_DIR" checkout -b code-branch
    echo "package main // updated" > "$TEST_DIR/main.go"
    echo "## Updated" >> "$TEST_DIR/README.md"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Update code and docs"

    changed=$(git -C "$TEST_DIR" diff --name-only main..code-branch)
    has_code=false
    while IFS= read -r file; do
        case "$file" in
            *.md|README*|LICENSE*|CHANGELOG*|*.txt|.gitignore) ;;
            *) has_code=true ;;
        esac
    done <<< "$changed"

    if [ "$has_code" = "true" ]; then
        log_pass "Correctly detected code changes alongside docs"
    else
        log_fail "Should have detected code changes"
    fi

    cleanup
    TEST_DIR=""
}

# -------------------------------------------------------------------
# Test 5: Clean branch passes all checks
# -------------------------------------------------------------------
test_clean_branch_passes() {
    log_test "Clean branch with tests and no deferred markers passes"
    setup_test_repo

    git -C "$TEST_DIR" checkout -b clean-branch
    mkdir -p "$TEST_DIR/pkg/feature"
    echo "package feature" > "$TEST_DIR/pkg/feature/feature.go"
    echo "package feature" > "$TEST_DIR/pkg/feature/feature_test.go"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Add feature with tests"

    # Check 1: has code changes
    changed=$(git -C "$TEST_DIR" diff --name-only main..clean-branch)
    has_code=false
    while IFS= read -r file; do
        case "$file" in
            *.md|README*|LICENSE*|CHANGELOG*|*.txt|.gitignore) ;;
            *) has_code=true ;;
        esac
    done <<< "$changed"

    # Check 2: no missing tests
    missing_tests=false
    while IFS= read -r file; do
        case "$file" in
            *_test.go) ;;
            *.go)
                dir="$TEST_DIR/$(dirname "$file")"
                if ! ls "$dir"/*_test.go >/dev/null 2>&1; then
                    missing_tests=true
                fi
                ;;
        esac
    done <<< "$changed"

    # Check 3: no deferred markers
    messages=$(git -C "$TEST_DIR" log --format="%s%n%b" main..clean-branch)
    has_deferred=false
    for pattern in "TODO" "FIXME" "HACK" "XXX" "deferred" "follow-up" "follow up" "punt" "skip for now"; do
        if echo "$messages" | grep -qi "$pattern"; then
            has_deferred=true
        fi
    done

    if [ "$has_code" = "true" ] && [ "$missing_tests" = "false" ] && [ "$has_deferred" = "false" ]; then
        log_pass "Clean branch correctly passes all checks"
    else
        log_fail "Clean branch should pass (code=$has_code, missing_tests=$missing_tests, deferred=$has_deferred)"
    fi

    cleanup
    TEST_DIR=""
}

# -------------------------------------------------------------------
# Test 6: Unmet acceptance criteria detection
# -------------------------------------------------------------------
test_unmet_acceptance_criteria() {
    log_test "Detect unmet acceptance criteria via keyword search"
    setup_test_repo

    git -C "$TEST_DIR" checkout -b partial-branch
    mkdir -p "$TEST_DIR/pkg/auth"
    echo "package auth" > "$TEST_DIR/pkg/auth/auth.go"
    echo "package auth" > "$TEST_DIR/pkg/auth/auth_test.go"
    git -C "$TEST_DIR" add -A
    git -C "$TEST_DIR" commit -m "Add authentication module"

    # Simulate acceptance criteria
    criteria_keywords=("authentication" "rate limiting" "logging")
    changed=$(git -C "$TEST_DIR" diff --name-only main..partial-branch)
    messages=$(git -C "$TEST_DIR" log --format="%s%n%b" main..partial-branch)
    all_text="$changed $messages"

    unmet=""
    for kw in "${criteria_keywords[@]}"; do
        if ! echo "$all_text" | grep -qi "$kw"; then
            unmet="${unmet}${kw}, "
        fi
    done

    if [ -n "$unmet" ]; then
        log_pass "Correctly detected unmet criteria: ${unmet%, }"
    else
        log_fail "Should have detected unmet criteria (rate limiting, logging)"
    fi

    cleanup
    TEST_DIR=""
}

# -------------------------------------------------------------------
# Run all tests
# -------------------------------------------------------------------
echo "==========================================="
echo "Audit Completion Skill - Test Suite"
echo "==========================================="
echo

test_docs_only_changes
test_missing_tests
test_deferred_markers
test_code_changes_detected
test_clean_branch_passes
test_unmet_acceptance_criteria

echo
echo "==========================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}$FAILED test(s) failed${NC}"
    exit 1
fi
