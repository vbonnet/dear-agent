#!/usr/bin/env bash
# Test suite for agm-exit bow gate enhancement
# Tests that agm-exit correctly integrates with /engram:bow as a pre-archive gate
# Does NOT test the Claude skill execution itself — tests the underlying logic

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
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

# -------------------------------------------------------------------
# Test 1: agm-exit.md declares bow as Step 0
# -------------------------------------------------------------------
test_exit_has_bow_step() {
    log_test "agm-exit.md includes bow as Step 0 (pre-archive gate)"

    skill_file="$SCRIPT_DIR/agm-exit.md"
    if [ ! -f "$skill_file" ]; then
        log_fail "agm-exit.md not found at $skill_file"
        return
    fi

    # Check for Step 0 with bow reference
    if grep -q "Step 0" "$skill_file"; then
        log_pass "agm-exit.md includes Step 0"
    else
        log_fail "agm-exit.md should include Step 0 (bow gate)"
    fi

    if grep -q "engram:bow" "$skill_file"; then
        log_pass "agm-exit.md references engram:bow skill"
    else
        log_fail "agm-exit.md should reference engram:bow skill"
    fi

    if grep -q "BLOCK" "$skill_file"; then
        log_pass "agm-exit.md includes BLOCK logic for critical failures"
    else
        log_fail "agm-exit.md should include BLOCK logic"
    fi
}

# -------------------------------------------------------------------
# Test 2: agm-exit.md blocks archive on test failures
# -------------------------------------------------------------------
test_exit_blocks_on_test_failure() {
    log_test "agm-exit.md blocks archive when tests fail"

    skill_file="$SCRIPT_DIR/agm-exit.md"

    # Check for test failure blocking
    if grep -q "Test failures.*BLOCK" "$skill_file"; then
        log_pass "agm-exit.md blocks on test failures"
    else
        log_fail "agm-exit.md should explicitly block on test failures"
    fi
}

# -------------------------------------------------------------------
# Test 3: agm-exit.md blocks archive on undone work
# -------------------------------------------------------------------
test_exit_blocks_on_undone_work() {
    log_test "agm-exit.md blocks archive when undone work detected"

    skill_file="$SCRIPT_DIR/agm-exit.md"

    # Check for undone work blocking
    if grep -q "Undone tasks.*BLOCK" "$skill_file"; then
        log_pass "agm-exit.md blocks on undone tasks"
    else
        log_fail "agm-exit.md should explicitly block on undone tasks"
    fi

    if grep -q "Broken promises.*BLOCK" "$skill_file"; then
        log_pass "agm-exit.md blocks on broken promises"
    else
        log_fail "agm-exit.md should explicitly block on broken promises"
    fi
}

# -------------------------------------------------------------------
# Test 4: agm-exit.md reports findings to orchestrator
# -------------------------------------------------------------------
test_exit_reports_to_orchestrator() {
    log_test "agm-exit.md reports bow findings to orchestrator via agm send msg"

    skill_file="$SCRIPT_DIR/agm-exit.md"

    if grep -q "agm send msg" "$skill_file"; then
        log_pass "agm-exit.md reports to orchestrator via agm send msg"
    else
        log_fail "agm-exit.md should report findings via agm send msg"
    fi

    if grep -q "orchestrator" "$skill_file"; then
        log_pass "agm-exit.md sends report to orchestrator"
    else
        log_fail "agm-exit.md should send report to orchestrator"
    fi
}

# -------------------------------------------------------------------
# Test 5: agm-exit.md declares required tools in allowed-tools
# -------------------------------------------------------------------
test_exit_allowed_tools() {
    log_test "agm-exit.md declares bow-related tools in allowed-tools"

    skill_file="$SCRIPT_DIR/agm-exit.md"

    # Check for Skill(engram:bow) in allowed-tools
    if grep -q "Skill(engram:bow)" "$skill_file"; then
        log_pass "agm-exit.md declares Skill(engram:bow) in allowed-tools"
    else
        log_fail "agm-exit.md should declare Skill(engram:bow) in allowed-tools"
    fi

    # Check for agm send msg in allowed-tools
    if grep -q "agm send msg" "$skill_file"; then
        log_pass "agm-exit.md allows agm send msg command"
    else
        log_fail "agm-exit.md should allow agm send msg command"
    fi
}

# -------------------------------------------------------------------
# Test 6: agm-exit.md allows warnings to proceed
# -------------------------------------------------------------------
test_exit_allows_warnings() {
    log_test "agm-exit.md allows WARNING-level issues to proceed (not block)"

    skill_file="$SCRIPT_DIR/agm-exit.md"

    if grep -q "WARNING-level" "$skill_file" && grep -q "continue to Step 1" "$skill_file"; then
        log_pass "agm-exit.md allows WARNING-level issues to proceed"
    else
        log_fail "agm-exit.md should allow WARNING-level issues to continue"
    fi
}

# -------------------------------------------------------------------
# Test 7: Bow gate runs BEFORE session name determination is needed
# -------------------------------------------------------------------
test_exit_step_ordering() {
    log_test "agm-exit.md runs bow gate (Step 0) before archive (Step 5)"

    skill_file="$SCRIPT_DIR/agm-exit.md"

    # Extract line numbers for Step 0 and Step 5
    step0_line=$(grep -n "Step 0" "$skill_file" | head -1 | cut -d: -f1)
    step5_line=$(grep -n "Step 5" "$skill_file" | head -1 | cut -d: -f1)

    if [ -n "$step0_line" ] && [ -n "$step5_line" ]; then
        if [ "$step0_line" -lt "$step5_line" ]; then
            log_pass "Step 0 (bow gate) comes before Step 5 (archive) — correct ordering"
        else
            log_fail "Step 0 should come before Step 5"
        fi
    else
        log_fail "Could not find Step 0 or Step 5 line numbers"
    fi
}

# -------------------------------------------------------------------
# Run all tests
# -------------------------------------------------------------------
echo "==========================================="
echo "AGM Exit Bow Gate Enhancement - Test Suite"
echo "==========================================="
echo

test_exit_has_bow_step
test_exit_blocks_on_test_failure
test_exit_blocks_on_undone_work
test_exit_reports_to_orchestrator
test_exit_allowed_tools
test_exit_allows_warnings
test_exit_step_ordering

echo
echo "==========================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}$FAILED test(s) failed${NC}"
    exit 1
fi
