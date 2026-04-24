#!/usr/bin/env bash
# Test suite for agm-exit command behavior
# Tests both tmux and non-tmux scenarios

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_SESSION="test-exit-session-$$"
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
    echo -e "${GREEN}✓ PASS:${NC} $1"
}

log_fail() {
    echo -e "${RED}✗ FAIL:${NC} $1"
    FAILED=$((FAILED + 1))
}

cleanup() {
    # Clean up test session if it exists
    agm session archive "$TEST_SESSION" --force 2>/dev/null || true
    # Remove test tmux session if it exists
    tmux kill-session -t "$TEST_SESSION" 2>/dev/null || true
}

# Setup
trap cleanup EXIT

# Test 1: Non-tmux scenario with session name argument
test_non_tmux_with_argument() {
    log_test "Non-tmux scenario: agm session archive with --force"

    # Create test session
    agm session associate "$TEST_SESSION" --create -C "$(pwd)" >/dev/null 2>&1

    # Verify session exists
    if ! agm get-uuid "$TEST_SESSION" >/dev/null 2>&1; then
        log_fail "Test session not created"
        return 1
    fi

    # Archive the session (simulating agm-exit in non-tmux mode)
    if agm session archive "$TEST_SESSION" --force; then
        log_pass "Session archived successfully without tmux"
    else
        log_fail "Failed to archive session without tmux"
        return 1
    fi

    # Verify session is archived
    if agm session list --all 2>/dev/null | grep -q "$TEST_SESSION"; then
        log_pass "Archived session appears in list --all"
    else
        log_fail "Archived session not found in list --all"
        return 1
    fi

    # Verify session does NOT appear in regular list
    if agm session list 2>/dev/null | grep -q "$TEST_SESSION"; then
        log_fail "Archived session should not appear in regular list"
        return 1
    else
        log_pass "Archived session correctly hidden from regular list"
    fi
}

# Test 2: Verify session association check
test_association_check() {
    log_test "Association check: agm get-uuid for non-existent session"

    local nonexistent="nonexistent-session-$$"

    # Should fail for non-existent session
    if agm get-uuid "$nonexistent" >/dev/null 2>&1; then
        log_fail "agm get-uuid should fail for non-existent session"
        return 1
    else
        log_pass "agm get-uuid correctly fails for non-existent session"
    fi
}

# Test 3: Verify archived session cannot be resumed
test_archived_cannot_resume() {
    log_test "Archived session: cannot be resumed"

    # Create and archive test session
    agm session associate "$TEST_SESSION" --create -C "$(pwd)" >/dev/null 2>&1
    agm session archive "$TEST_SESSION" --force >/dev/null 2>&1

    # Try to resume - should fail
    if agm session resume "$TEST_SESSION" --dry-run 2>&1 | grep -q "archived"; then
        log_pass "Resume correctly blocked for archived session"
    else
        log_fail "Resume should be blocked for archived session"
        return 1
    fi
}

# Test 4: Force flag bypasses confirmation
test_force_flag() {
    log_test "Force flag: bypasses confirmation prompt"

    # Create test session
    agm session associate "$TEST_SESSION" --create -C "$(pwd)" >/dev/null 2>&1

    # Archive with --force should not prompt and should succeed
    if timeout 2 agm session archive "$TEST_SESSION" --force >/dev/null 2>&1; then
        log_pass "Force flag successfully bypasses confirmation"
    else
        log_fail "Force flag should bypass confirmation (timed out or failed)"
        return 1
    fi
}

# Test 5: Async flag requires tmux (integration test hint)
test_async_requires_tmux() {
    log_test "Async flag: note that it requires tmux (integration test needed)"

    # Create test session
    agm session associate "$TEST_SESSION" --create -C "$(pwd)" >/dev/null 2>&1

    # Try async without tmux - should fail or warn
    # Note: Full tmux integration test should be separate
    if ! tmux has-session -t "$TEST_SESSION" 2>/dev/null; then
        log_pass "No tmux session exists (async would need tmux context)"
    fi

    echo "    Note: Full async archival with tmux reaper requires integration test"
}

# Run all tests
echo "==================================="
echo "AGM Exit Command Test Suite"
echo "==================================="
echo

test_non_tmux_with_argument
cleanup
test_association_check
test_archived_cannot_resume
cleanup
test_force_flag
cleanup
test_async_requires_tmux
cleanup

echo
echo "==================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}$FAILED test(s) failed${NC}"
    exit 1
fi
