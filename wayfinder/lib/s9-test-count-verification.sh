#!/bin/bash
set -euo pipefail

# s9-test-count-verification.sh: S9 Test Count Verification (for test beads)
# Prevents fabricated test coverage claims by verifying actual test execution
# Exit 0 = pass, Exit 1 = fail (blocks S9 completion)

PROJECT_DIR="${1:-.}"
S8_DELIVERABLE="$PROJECT_DIR/S8-implementation.md"
S9_DELIVERABLE="$PROJECT_DIR/S9-validation.md"

# Skip if not a test bead
IS_TEST_BEAD=false
if [[ -f "$PROJECT_DIR/W0-charter.md" ]] || [[ -f "$PROJECT_DIR/W0-project-charter.md" ]]; then
    CHARTER_FILE=$(find "$PROJECT_DIR" -maxdepth 1 -name "W0-*.md" | head -1)
    if [[ -n "$CHARTER_FILE" ]]; then
        if grep -qi "test suite\|testing\|test.*implementation\|add.*tests" "$CHARTER_FILE" 2>/dev/null; then
            IS_TEST_BEAD=true
        fi
    fi
fi

if [[ "$IS_TEST_BEAD" != "true" ]]; then
    echo "ℹ️  Not a test bead, skipping test count verification"
    exit 0
fi

echo "🔍 S9 Test Count Verification: Validating test execution metrics..."
echo ""

# ============================================================================
# Check 1: Extract Claimed Counts from S8/S9 Deliverables
# ============================================================================

CLAIMED_NEW_TESTS=0
CLAIMED_TOTAL_TESTS=0
CLAIMED_BASELINE_TESTS=0

if [[ -f "$S8_DELIVERABLE" ]]; then
    # Look for patterns like "45 new tests" or "+46 tests"
    NEW_TEST_CLAIM=$(grep -oiE "[+]?[0-9]+ new tests?" "$S8_DELIVERABLE" | grep -oE "[0-9]+" | head -1)
    if [[ -n "$NEW_TEST_CLAIM" ]]; then
        CLAIMED_NEW_TESTS=$NEW_TEST_CLAIM
    fi

    # Look for total test count like "244 tests" or "260 tests passing"
    TOTAL_TEST_CLAIM=$(grep -oiE "[0-9]+ tests? (passing|total)" "$S8_DELIVERABLE" | grep -oE "[0-9]+" | head -1)
    if [[ -n "$TOTAL_TEST_CLAIM" ]]; then
        CLAIMED_TOTAL_TESTS=$TOTAL_TEST_CLAIM
    fi
fi

if [[ -f "$S9_DELIVERABLE" ]]; then
    # S9 should have validation results
    S9_TOTAL=$(grep -oiE "Tests?:.*[0-9]+ passed" "$S9_DELIVERABLE" | grep -oE "[0-9]+" | head -1)
    if [[ -n "$S9_TOTAL" ]]; then
        CLAIMED_TOTAL_TESTS=$S9_TOTAL
    fi

    S9_NEW=$(grep -oiE "[+][0-9]+ (new )?tests?" "$S9_DELIVERABLE" | grep -oE "[0-9]+" | head -1)
    if [[ -n "$S9_NEW" ]]; then
        CLAIMED_NEW_TESTS=$S9_NEW
    fi

    # Extract baseline from patterns like "214 baseline + 46 new"
    BASELINE_CLAIM=$(grep -oiE "[0-9]+ baseline" "$S9_DELIVERABLE" | grep -oE "[0-9]+" | head -1)
    if [[ -n "$BASELINE_CLAIM" ]]; then
        CLAIMED_BASELINE_TESTS=$BASELINE_CLAIM
    fi
fi

echo "📊 Claimed Metrics (from S8/S9 docs):"
echo "  Baseline tests: ${CLAIMED_BASELINE_TESTS:-unknown}"
echo "  New tests claimed: ${CLAIMED_NEW_TESTS:-unknown}"
echo "  Total tests claimed: ${CLAIMED_TOTAL_TESTS:-unknown}"
echo ""

# ============================================================================
# Check 2: Run Actual Tests and Count Results
# ============================================================================

ACTUAL_TEST_COUNT=0
TEST_OUTPUT_FILE="$PROJECT_DIR/S9-test-execution.log"

echo "🧪 Running actual test suite to verify claims..."

# Auto-detect test framework and run tests
if [[ -f "$PROJECT_DIR/package.json" ]]; then
    # JavaScript/TypeScript with Jest/Mocha
    echo "  Detected: npm project (running npm test)"
    if npm test --prefix "$PROJECT_DIR" > "$TEST_OUTPUT_FILE" 2>&1; then
        # Parse Jest output: "Tests: 260 passed, 260 total"
        ACTUAL_TEST_COUNT=$(grep -oE "Tests:.*[0-9]+ passed" "$TEST_OUTPUT_FILE" | grep -oE "[0-9]+" | head -1)
        if [[ -z "$ACTUAL_TEST_COUNT" ]]; then
            # Try alternative format: "260 passing"
            ACTUAL_TEST_COUNT=$(grep -oE "[0-9]+ passing" "$TEST_OUTPUT_FILE" | grep -oE "[0-9]+" | head -1)
        fi
    else
        echo "❌ S9 Verification Failed: Tests failed to run"
        echo ""
        echo "Test execution failed. Review output:"
        tail -20 "$TEST_OUTPUT_FILE"
        echo ""
        echo "Cannot verify test count claims if tests don't pass."
        exit 1
    fi

elif [[ -f "$PROJECT_DIR/go.mod" ]]; then
    # Go project
    echo "  Detected: Go project (running go test)"
    if go test ./... -v > "$TEST_OUTPUT_FILE" 2>&1; then
        # Count PASS lines
        ACTUAL_TEST_COUNT=$(grep -c "^--- PASS" "$TEST_OUTPUT_FILE")
    else
        echo "❌ S9 Verification Failed: Go tests failed"
        tail -20 "$TEST_OUTPUT_FILE"
        exit 1
    fi

elif find "$PROJECT_DIR" -name "test_*.py" -o -name "*_test.py" | grep -q .; then
    # Python project
    echo "  Detected: Python project (running pytest)"
    if pytest "$PROJECT_DIR" -v > "$TEST_OUTPUT_FILE" 2>&1; then
        # Parse pytest output: "260 passed"
        ACTUAL_TEST_COUNT=$(grep -oE "[0-9]+ passed" "$TEST_OUTPUT_FILE" | grep -oE "[0-9]+" | head -1)
    else
        echo "❌ S9 Verification Failed: pytest failed"
        tail -20 "$TEST_OUTPUT_FILE"
        exit 1
    fi

else
    echo "⚠️  Warning: Could not auto-detect test framework"
    echo "  Skipping automated test count verification"
    echo "  Manually verify test counts match claims in S8/S9 documents"
    exit 0
fi

echo ""
echo "📊 Actual Test Execution Results:"
echo "  Tests executed: ${ACTUAL_TEST_COUNT:-0}"
echo ""

# ============================================================================
# Check 3: Verify Claims Match Reality
# ============================================================================

VERIFICATION_FAILED=false

if [[ "$CLAIMED_TOTAL_TESTS" -gt 0 ]] && [[ "${ACTUAL_TEST_COUNT:-0}" -gt 0 ]]; then
    if [[ "$CLAIMED_TOTAL_TESTS" -ne "$ACTUAL_TEST_COUNT" ]]; then
        echo "❌ CRITICAL: Test count mismatch detected"
        echo ""
        echo "Claimed in S8/S9: $CLAIMED_TOTAL_TESTS tests"
        echo "Actual execution: $ACTUAL_TEST_COUNT tests"
        echo "Discrepancy: $((CLAIMED_TOTAL_TESTS - ACTUAL_TEST_COUNT)) tests"
        echo ""
        VERIFICATION_FAILED=true
    fi
fi

# Check if new tests were actually created
if [[ "$CLAIMED_NEW_TESTS" -gt 0 ]]; then
    # Count test files created in this session (git diff)
    if git -C "$PROJECT_DIR" rev-parse --git-dir > /dev/null 2>&1; then
        NEW_TEST_FILES=$(git -C "$PROJECT_DIR" diff --name-only --diff-filter=A HEAD~5..HEAD | grep -E "\.(test|spec)\.(ts|js|py|go|rs)$|_test\.(go|py|rs)$|^test_.*\.py$" | wc -l)

        if [[ "$NEW_TEST_FILES" -eq 0 ]]; then
            echo "❌ CRITICAL: Claimed $CLAIMED_NEW_TESTS new tests but no new test files in git history"
            echo ""
            echo "Claimed: $CLAIMED_NEW_TESTS new tests"
            echo "Git history: 0 new test files"
            echo ""
            VERIFICATION_FAILED=true
        fi
    fi
fi

# ============================================================================
# Check 4: Historical Pattern Detection (oss-n1nq.12 pattern)
# ============================================================================

if [[ "$VERIFICATION_FAILED" == "true" ]]; then
    echo "PATTERN MATCH: oss-n1nq.12 (Fabricated Metrics)"
    echo ""
    echo "Historical failure (oss-n1nq.12 - Auth Test Suite):"
    echo "  - Claimed: 90.54% coverage, 45 new tests, 244 total tests"
    echo "  - Reality: 0 test files created, 0 commits, 214/214 tests unchanged"
    echo "  - Outcome: Failed Gate 3, rewound S11 → S7"
    echo ""
    echo "Your project shows similar pattern:"
    echo "  - S8/S9 documents claim test counts"
    echo "  - Actual test execution shows different count"
    echo "  - Suggests fabricated metrics without real implementation"
    echo ""
    echo "Remediation:"
    echo "  1. Review S8-implementation.md for 'conceptual' or 'demonstration' markers"
    echo "  2. Check if actual test files exist (not just documented in markdown)"
    echo "  3. Verify git commits show test file creation"
    echo "  4. Re-run tests and update S9 with ACTUAL results"
    echo ""
    echo "Reference: ~/src/engram/plugins/wayfinder/engrams/workflows/s9-validation.ai.md"
    echo ""
    exit 1
fi

# ============================================================================
# Success
# ============================================================================

echo "✅ S9 Test Count Verification Passed"
echo ""
echo "Test execution verified:"
echo "  - Actual test count matches claimed count"
echo "  - Tests executed successfully"
echo "  - No fabricated metrics detected"
echo ""

exit 0
