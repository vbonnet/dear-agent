#!/bin/bash
set -euo pipefail

# s8-mid-phase-checkpoint.sh: Mid-phase validation during S8 Implementation
# Catches agents writing planning docs instead of code BEFORE they finish S8
# Exit 0 = informational (always succeeds, but shows warnings)
# This is different from s8-gate-check.sh which BLOCKS completion

PROJECT_DIR="${1:-.}"

echo "🔍 S8 Mid-Phase Checkpoint"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# ============================================================================
# Check 1: Code Files Count
# ============================================================================

CODE_PATTERNS=(-name "*.go" -o -name "*.py" -o -name "*.ts" -o -name "*.js" -o -name "*.rs" -o -name "*.java" -o -name "*.rb" -o -name "*.php" -o -name "*.c" -o -name "*.cpp" -o -name "*.h" -o -name "*.hpp")

CODE_FILES=$(find "$PROJECT_DIR" \
    -type f \
    \( "${CODE_PATTERNS[@]}" \) \
    ! -path "*/node_modules/*" \
    ! -path "*/vendor/*" \
    ! -path "*/.git/*" \
    ! -path "*/dist/*" \
    ! -path "*/build/*" \
    2>/dev/null || echo "")

CODE_COUNT=$(echo "$CODE_FILES" | grep -c . || echo "0")

# ============================================================================
# Check 2: Test Files Count
# ============================================================================

TEST_PATTERNS=(-name "*_test.go" -o -name "*_test.py" -o -name "test_*.py" -o -name "*.test.ts" -o -name "*.test.js" -o -name "*.spec.ts" -o -name "*.spec.js" -o -name "*_test.rs")

TEST_FILES=$(find "$PROJECT_DIR" \
    -type f \
    \( "${TEST_PATTERNS[@]}" \) \
    ! -path "*/node_modules/*" \
    ! -path "*/vendor/*" \
    ! -path "*/.git/*" \
    2>/dev/null || echo "")

TEST_COUNT=$(echo "$TEST_FILES" | grep -c . || echo "0")

# ============================================================================
# Check 3: Git Commits Count
# ============================================================================

COMMIT_COUNT=0
if git -C "$PROJECT_DIR" rev-parse --git-dir > /dev/null 2>&1; then
    # Count commits since S8 phase started (approximate: commits in last hour)
    COMMIT_COUNT=$(git -C "$PROJECT_DIR" log --oneline --since="1 hour ago" 2>/dev/null | wc -l || echo "0")
fi

# ============================================================================
# Check 4: Red Flag Detection
# ============================================================================

RED_FLAGS_FOUND=false
RED_FLAG_LIST=""

# Check S8-implementation.md for red flags
if [[ -f "$PROJECT_DIR/S8-implementation.md" ]]; then
    RED_FLAG_PATTERNS=("would implement" "demonstration" "blueprint" "conceptual" "ready for implementation" "what would be" "example implementation")

    for pattern in "${RED_FLAG_PATTERNS[@]}"; do
        if grep -qi "$pattern" "$PROJECT_DIR/S8-implementation.md"; then
            RED_FLAGS_FOUND=true
            RED_FLAG_LIST="${RED_FLAG_LIST}- \"$pattern\" found in S8-implementation.md\n"
        fi
    done
fi

# ============================================================================
# Display Results
# ============================================================================

echo "📊 Current Progress:"
echo "  Code files:   $CODE_COUNT"
echo "  Test files:   $TEST_COUNT"
echo "  Git commits:  $COMMIT_COUNT (in last hour)"
echo ""

# ============================================================================
# Warnings and Recommendations
# ============================================================================

WARNING_SHOWN=false

# Warning 1: Zero code files
if [[ "$CODE_COUNT" -eq 0 ]]; then
    WARNING_SHOWN=true
    echo "⚠️  WARNING: Zero code files detected"
    echo ""
    echo "You should have created at least 1 source file by now."
    echo ""
    echo "Expected files:"
    echo "  ✅ main.go, server.py, app.ts, handler.js"
    echo "  ❌ S8-implementation.md (that's a planning document, not code)"
    echo ""
    echo "Action: Create actual code files NOW, don't wait until end of S8"
    echo ""
fi

# Warning 2: Zero commits
if [[ "$COMMIT_COUNT" -eq 0 ]] && [[ "$CODE_COUNT" -gt 0 ]]; then
    WARNING_SHOWN=true
    echo "⚠️  WARNING: Code exists but zero commits"
    echo ""
    echo "You have $CODE_COUNT code files but haven't committed anything."
    echo ""
    echo "S8 requires incremental commits as you implement."
    echo ""
    echo "Action: Commit your work now:"
    echo "  git add ."
    echo "  git commit -m 'wayfinder: implement [component name]'"
    echo ""
fi

# Warning 3: Red flags detected
if [[ "$RED_FLAGS_FOUND" == "true" ]]; then
    WARNING_SHOWN=true
    echo "🚨 CRITICAL: Design document language detected"
    echo ""
    echo "Red flags found:"
    echo -e "$RED_FLAG_LIST"
    echo ""
    echo "You are writing a PLANNING DOCUMENT (S7), not IMPLEMENTING CODE (S8)."
    echo ""
    echo "Action required:"
    echo "  1. Delete S8-implementation.md (it's a planning doc)"
    echo "  2. Create actual .go, .py, .ts, .js files with real code"
    echo "  3. Write tests and commit them to git"
    echo "  4. Re-run this checkpoint to verify"
    echo ""
fi

# Warning 4: Test-focused bead with no tests
if [[ -f "$PROJECT_DIR/W0-charter.md" ]] || [[ -f "$PROJECT_DIR/W0-project-charter.md" ]]; then
    CHARTER_FILE=$(find "$PROJECT_DIR" -maxdepth 1 -name "W0-*.md" | head -1)
    if [[ -n "$CHARTER_FILE" ]]; then
        if grep -qi "test suite\|testing\|test.*implementation\|add.*tests" "$CHARTER_FILE" 2>/dev/null; then
            if [[ "$TEST_COUNT" -eq 0 ]]; then
                WARNING_SHOWN=true
                echo "⚠️  WARNING: Test-focused bead with zero test files"
                echo ""
                echo "This bead is about implementing TESTS, but no test files exist."
                echo ""
                echo "Expected test files:"
                echo "  ✅ *_test.go, test_*.py, *.test.ts, *.spec.js"
                echo "  ❌ Documentation about tests (not actual test code)"
                echo ""
                echo "Action: Create test files NOW with actual test cases"
                echo ""
            fi
        fi
    fi
fi

# ============================================================================
# Success Path
# ============================================================================

if [[ "$WARNING_SHOWN" == "false" ]]; then
    echo "✅ Checkpoint PASSED"
    echo ""
    echo "Good progress:"
    echo "  - Code files exist: $CODE_COUNT"
    if [[ "$TEST_COUNT" -gt 0 ]]; then
        echo "  - Tests exist: $TEST_COUNT"
    fi
    if [[ "$COMMIT_COUNT" -gt 0 ]]; then
        echo "  - Commits made: $COMMIT_COUNT"
    fi
    echo "  - No red flags detected"
    echo ""
    echo "Continue implementing remaining components."
    echo "Run this checkpoint again after next component."
    echo ""
fi

# ============================================================================
# Reminder
# ============================================================================

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "This is a mid-phase check. Run again after each component."
echo "Final validation happens at end: s8-gate-check.sh"
echo ""

exit 0
