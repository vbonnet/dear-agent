#!/bin/bash
set -e

echo "========================================="
echo "   AGM Reaper E2E Test Suite"
echo "========================================="
echo ""

# Track results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Helper function to run a test
run_test() {
    local test_name="$1"
    local test_script="$2"

    echo ""
    echo ">>> Running: $test_name"
    echo ""

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    if bash "$test_script"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
        echo ""
        echo "✓ $test_name PASSED"
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        echo ""
        echo "✗ $test_name FAILED"
    fi

    echo "========================================="
}

# Run tests
run_test "Reaper Happy Path" "/home/testuser/tests/test_reaper_happy_path.sh"
run_test "Reaper Binary Missing" "/home/testuser/tests/test_reaper_binary_missing.sh"

# ~4.5 min: 90s prompt detection + 60s fallback + the pane-close escalation.
# It was commented out "for faster CI" and rotted unnoticed behind that comment
# — by the time anyone looked it had three separate breakages. The job's budget
# is 30 minutes and the whole suite is under 6, so the saving never justified
# leaving the reaper's timeout path uncovered.
run_test "Reaper Prompt Timeout" "/home/testuser/tests/test_reaper_prompt_timeout.sh"

# Future tests (uncomment when implemented):
# run_test "AGM Exit Workflow" "/home/testuser/tests/test_agm_exit_workflow.sh"

# Summary
echo ""
echo "========================================="
echo "   Test Summary"
echo "========================================="
echo "Total:  $TOTAL_TESTS"
echo "Passed: $PASSED_TESTS"
echo "Failed: $FAILED_TESTS"
echo "========================================="

if [ $FAILED_TESTS -eq 0 ]; then
    echo ""
    echo "🎉 All tests passed!"
    exit 0
else
    echo ""
    echo "❌ Some tests failed"
    exit 1
fi
