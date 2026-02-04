#!/bin/bash
# Run all astrocyte E2E tests in Docker container
# Exit with 0 if all tests pass, non-zero otherwise

set -euo pipefail

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
}

log_section() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "$*"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# Test tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
declare -a FAILED_TEST_NAMES

# Run a single test
run_test() {
    local test_script="$1"
    local test_name=$(basename "$test_script" .sh)

    ((TOTAL_TESTS++))

    log_section "Running: $test_name"

    if bash "$test_script"; then
        log_pass "✓ $test_name PASSED"
        ((PASSED_TESTS++))
        return 0
    else
        log_fail "✗ $test_name FAILED"
        ((FAILED_TESTS++))
        FAILED_TEST_NAMES+=("$test_name")
        return 1
    fi
}

# Main test suite
main() {
    log_section "Astrocyte E2E Test Suite"
    echo "Starting test execution..."
    echo "Timestamp: $(date -Iseconds)"
    echo ""

    # List of tests to run (in order)
    TESTS=(
        "/tests/test_mustering_detection.sh"
        "/tests/test_zero_token_recovery.sh"
        "/tests/test_permission_rejection.sh"
        "/tests/test_multi_session.sh"
        "/tests/test_incident_logging.sh"
    )

    # Run each test
    for test in "${TESTS[@]}"; do
        if [ -f "$test" ]; then
            run_test "$test" || true  # Continue even if test fails
        else
            log_fail "Test not found: $test"
            ((FAILED_TESTS++))
        fi

        # Add separator between tests
        echo ""
    done

    # Final summary
    log_section "Test Summary"
    echo ""
    echo "Total Tests:  $TOTAL_TESTS"
    echo "Passed:       $PASSED_TESTS"
    echo "Failed:       $FAILED_TESTS"
    echo ""

    if [ "$FAILED_TESTS" -eq 0 ]; then
        log_pass "╔═══════════════════════════════════════════════════════════╗"
        log_pass "║          ALL TESTS PASSED ✓                               ║"
        log_pass "╚═══════════════════════════════════════════════════════════╝"
        exit 0
    else
        log_fail "╔═══════════════════════════════════════════════════════════╗"
        log_fail "║          SOME TESTS FAILED ✗                              ║"
        log_fail "╚═══════════════════════════════════════════════════════════╝"
        echo ""
        echo "Failed tests:"
        for test_name in "${FAILED_TEST_NAMES[@]}"; do
            echo "  - $test_name"
        done
        echo ""
        exit 1
    fi
}

main "$@"
