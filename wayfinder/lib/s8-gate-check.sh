#!/bin/bash
set -euo pipefail

# s8-gate-check.sh: S8 Implementation validation
# Ensures actual code files created, not just planning documents
# Exit 0 = pass, Exit 1 = fail (blocks S8 completion)

PROJECT_DIR="${1:-.}"
S8_DELIVERABLE="$PROJECT_DIR/S8-implementation.md"

# Exit silently if S8 deliverable doesn't exist yet
if [[ ! -f "$S8_DELIVERABLE" ]]; then
    exit 0
fi

echo "🔍 S8 Gate Check: Validating implementation files..."
echo ""

# ============================================================================
# Check 1: Code Files Exist
# ============================================================================

# Find source code files (exclude common non-source directories)
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
# Check 2: Test Files Exist (for test-focused beads)
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
# Check 3: Bead Type Detection (optional doc-only projects)
# ============================================================================

IS_DOC_ONLY=false
IS_TEST_BEAD=false

if [[ -f "$PROJECT_DIR/W0-charter.md" ]] || [[ -f "$PROJECT_DIR/W0-project-charter.md" ]]; then
    CHARTER_FILE=$(find "$PROJECT_DIR" -maxdepth 1 -name "W0-*.md" | head -1)
    if [[ -n "$CHARTER_FILE" ]]; then
        # Check for documentation-only indicators
        if grep -qi "type.*documentation\|documentation.*only\|docs.*only" "$CHARTER_FILE" 2>/dev/null; then
            IS_DOC_ONLY=true
        fi

        # Check for test-focused indicators
        if grep -qi "test suite\|testing\|test.*implementation\|add.*tests" "$CHARTER_FILE" 2>/dev/null; then
            IS_TEST_BEAD=true
        fi
    fi
fi

# ============================================================================
# Check 4: Git Commit Status
# ============================================================================

UNCOMMITTED_COUNT=0
GIT_STATUS=""

if git -C "$PROJECT_DIR" rev-parse --git-dir > /dev/null 2>&1; then
    GIT_STATUS=$(git -C "$PROJECT_DIR" status --porcelain 2>/dev/null || echo "")
    UNCOMMITTED_COUNT=$(echo "$GIT_STATUS" | grep -c . || echo "0")
fi

# ============================================================================
# Validation Logic
# ============================================================================

# Display findings
echo "📊 Validation Results:"
echo "  Code files: $CODE_COUNT"
echo "  Test files: $TEST_COUNT"
echo "  Project type: $(if [[ "$IS_DOC_ONLY" == "true" ]]; then echo "Documentation-only"; elif [[ "$IS_TEST_BEAD" == "true" ]]; then echo "Test-focused"; else echo "Code implementation"; fi)"

if [[ -n "$GIT_STATUS" ]]; then
    echo "  Git status: $UNCOMMITTED_COUNT uncommitted file(s)"
else
    echo "  Git status: Not a git repository (skipping)"
fi

echo ""

# ============================================================================
# Gate Validation: Code Files Required
# ============================================================================

if [[ "$CODE_COUNT" -eq 0 ]] && [[ "$IS_DOC_ONLY" != "true" ]]; then
    echo "❌ S8 Gate Failed: No code files created"
    echo ""
    echo "S8 (Implementation) requires ACTUAL source code files, not planning documents."
    echo ""
    echo "Expected file types:"
    echo "  - *.go (Go source)"
    echo "  - *.py (Python source)"
    echo "  - *.ts, *.js (TypeScript/JavaScript source)"
    echo "  - *.rs (Rust source)"
    echo "  - *.java (Java source)"
    echo "  - *.rb (Ruby source)"
    echo "  - *.php (PHP source)"
    echo "  - *.c, *.cpp (C/C++ source)"
    echo ""
    echo "Found: 0 source files"
    echo ""
    echo "Common mistake: Creating 'S8-implementation.md' with code snippets"
    echo "  ❌ WRONG: S8-implementation.md with \`\`\`python code blocks"
    echo "  ✅ CORRECT: actual_code.py with executable Python code"
    echo ""
    echo "Remediation steps:"
    echo "  1. Create actual code files implementing S7 plan"
    echo "  2. Place files in appropriate directories (src/, lib/, cmd/, etc.)"
    echo "  3. Write tests for the code (*_test.*, test_*, *.test.*)"
    echo "  4. Commit files: git add . && git commit -m 'Implement S8'"
    echo "  5. Re-run S8 validation"
    echo ""
    echo "Reference: ~/src/engram/plugins/wayfinder/engrams/workflows/s8-implementation.ai.md"
    echo ""
    exit 1
fi

# ============================================================================
# Gate Validation: Test Files Required (for test beads)
# ============================================================================

if [[ "$IS_TEST_BEAD" == "true" ]] && [[ "$TEST_COUNT" -eq 0 ]]; then
    echo "❌ S8 Gate Failed: No test files created (test-focused bead)"
    echo ""
    echo "This bead is focused on implementing tests, but no test files were found."
    echo ""
    echo "Expected test file patterns:"
    echo "  - *_test.go (Go tests)"
    echo "  - *_test.py, test_*.py (Python tests)"
    echo "  - *.test.ts, *.test.js (TypeScript/JavaScript tests)"
    echo "  - *.spec.ts, *.spec.js (Jest/spec tests)"
    echo "  - *_test.rs (Rust tests)"
    echo ""
    echo "Found: 0 test files"
    echo ""
    echo "Remediation steps:"
    echo "  1. Create test files with actual test cases"
    echo "  2. Ensure tests are executable (not just examples in markdown)"
    echo "  3. Run tests locally to verify they work"
    echo "  4. Commit test files: git add . && git commit -m 'Add tests'"
    echo "  5. Re-run S8 validation"
    echo ""
    exit 1
fi

# ============================================================================
# Check 5: Red Flag Detection (Design Docs Instead of Code)
# ============================================================================

if [[ -f "$S8_DELIVERABLE" ]]; then
    # Scan S8 deliverable for red flag patterns
    RED_FLAGS_FOUND=false
    RED_FLAG_PATTERNS=("would implement" "demonstration" "blueprint" "conceptual" "ready for implementation" "what would be" "example implementation")

    for pattern in "${RED_FLAG_PATTERNS[@]}"; do
        if grep -qi "$pattern" "$S8_DELIVERABLE"; then
            if [[ "$RED_FLAGS_FOUND" == "false" ]]; then
                echo "❌ S8 Gate Failed: Design document detected instead of implementation"
                echo ""
                echo "Red flag patterns found in S8-implementation.md:"
                echo ""
                RED_FLAGS_FOUND=true
            fi

            # Show first occurrence with context
            grep -ni "$pattern" "$S8_DELIVERABLE" | head -3 | while IFS=: read -r line_num line_text; do
                echo "  Line $line_num: \"$pattern\" - $line_text"
            done
            echo ""
        fi
    done

    if [[ "$RED_FLAGS_FOUND" == "true" ]]; then
        echo "CRITICAL VIOLATION:"
        echo "  S8 (Implementation) requires ACTUAL source code, not planning documents."
        echo "  Your S8-implementation.md contains design/planning language."
        echo ""
        echo "Common pattern:"
        echo "  ❌ WRONG: S8-implementation.md with 'What Would Be Implemented' sections"
        echo "  ✅ CORRECT: Actual .ts, .py, .go files with working code"
        echo ""
        echo "Historical failures (learn from these):"
        echo "  - oss-n1nq.12: Claimed 90.54% coverage, delivered 0 tests (fabricated metrics)"
        echo "  - oss-n1nq.11: Delivered assessment docs instead of test files"
        echo "  - Pattern: Agents complete S8-S11 but deliver design docs, not code"
        echo ""
        echo "Remediation:"
        echo "  1. Delete S8-implementation.md (it's a planning document)"
        echo "  2. Create actual source code files (*.ts, *.py, *.go)"
        echo "  3. Write tests and run them locally"
        echo "  4. Commit implementation: git add . && git commit -m 'Implement S8'"
        echo "  5. Re-run S8 gate validation"
        echo ""
        echo "Reference: ~/src/engram/plugins/wayfinder/engrams/workflows/s8-implementation.ai.md"
        echo "           Lines 13-64: 'ULTRA-EXPLICIT S8 REQUIREMENTS'"
        echo ""
        exit 1
    fi
fi

# ============================================================================
# Warning: Uncommitted Files
# ============================================================================

if [[ "$UNCOMMITTED_COUNT" -gt 0 ]]; then
    echo "⚠️  Warning: $UNCOMMITTED_COUNT uncommitted file(s) detected"
    echo ""
    echo "Git status:"
    echo "$GIT_STATUS" | head -10
    if [[ "$UNCOMMITTED_COUNT" -gt 10 ]]; then
        echo "  ... and $((UNCOMMITTED_COUNT - 10)) more"
    fi
    echo ""
    echo "Recommendation:"
    echo "  Commit your implementation before completing S8:"
    echo "  git add ."
    echo "  git commit -m 'wayfinder: implement S8'"
    echo ""
    echo "Note: This is a warning, not a blocker. Continuing..."
    echo ""
fi

# ============================================================================
# Success
# ============================================================================

echo "✅ S8 Gate Passed"
echo ""
echo "Implementation validated:"
echo "  - $CODE_COUNT code file(s) created"
if [[ "$TEST_COUNT" -gt 0 ]]; then
    echo "  - $TEST_COUNT test file(s) created"
fi
echo ""

exit 0
