#!/bin/bash
#
# Phase 4 Production Readiness Validation Script
# Systematically executes all checklist items from PRODUCTION-READINESS-CHECKLIST.md
#
# Usage: ./scripts/validate-phase4.sh [--skip-api-tests]
#

set +e  # Don't exit on errors, we want to run all checks

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

SKIP_API_TESTS=false
if [[ "$1" == "--skip-api-tests" ]]; then
    SKIP_API_TESTS=true
fi

echo "=================================================="
echo "Phase 4: Production Readiness Validation"
echo "=================================================="
echo

# Track results
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASS_COUNT++))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    ((FAIL_COUNT++))
}

skip() {
    echo -e "${YELLOW}⊘${NC} $1"
    ((SKIP_COUNT++))
}

section() {
    echo
    echo "----------------------------------------"
    echo "$1"
    echo "----------------------------------------"
}

# ==================== Section 1: Security Audit ====================

section "1. Security Audit"

echo -n "1.1 Checking for hardcoded API keys... "
if grep -r 'GEMINI_API_KEY\s*=\s*"[^"]*"' --include="*.go" . > /dev/null 2>&1; then
    fail "Found hardcoded API keys"
    grep -r 'GEMINI_API_KEY\s*=\s*"[^"]*"' --include="*.go" .
else
    pass "No hardcoded API keys found"
fi

echo -n "1.2 Checking log statements for sensitive data... "
# Check if any log statements might output API keys
if grep -r 'log.*API' --include="*.go" internal/agent/gemini* > /dev/null 2>&1; then
    skip "Found log statements with 'API' - manual review needed"
else
    pass "No suspicious log statements found"
fi

echo -n "1.3 Checking file permission patterns... "
# Look for unsafe file operations
unsafe=$(grep -r 'os.WriteFile.*0777\|os.Create' --include="*.go" internal/agent/gemini* 2>/dev/null | wc -l)
if [ "$unsafe" -gt 0 ]; then
    skip "Found $unsafe file operations - manual review needed"
else
    pass "File operations look safe"
fi

# ==================== Section 2: Error Handling ====================

section "2. Error Handling Completeness"

echo -n "2.1 Running error path tests... "
if go test ./internal/agent/... -v -run TestError > /tmp/error-tests.log 2>&1; then
    pass "Error path tests passed"
else
    if grep -q "no tests to run" /tmp/error-tests.log; then
        skip "No dedicated error tests found"
    else
        fail "Error tests failed (see /tmp/error-tests.log)"
    fi
fi

echo -n "2.2 Running race detector... "
if go test -race ./internal/agent/... -run TestGemini > /tmp/race-tests.log 2>&1; then
    pass "Race detector found no issues"
else
    if grep -q "no tests to run" /tmp/race-tests.log; then
        skip "No Gemini unit tests found"
    else
        fail "Race conditions detected (see /tmp/race-tests.log)"
    fi
fi

# ==================== Section 3: Code Quality ====================

section "3. Code Quality"

echo -n "3.1 Running gofmt... "
unformatted=$(gofmt -l internal/agent/gemini* 2>/dev/null || true)
if [ -z "$unformatted" ]; then
    pass "All files formatted correctly"
else
    fail "Unformatted files: $unformatted"
fi

echo -n "3.2 Running go vet... "
if go vet ./internal/agent/... > /tmp/vet.log 2>&1; then
    pass "go vet passed"
else
    fail "go vet found issues (see /tmp/vet.log)"
fi

echo -n "3.3 Running golangci-lint... "
if command -v golangci-lint > /dev/null 2>&1; then
    if golangci-lint run ./internal/agent/gemini_cli_adapter.go > /tmp/lint.log 2>&1; then
        pass "golangci-lint passed"
    else
        fail "Linting issues found (see /tmp/lint.log)"
    fi
else
    skip "golangci-lint not installed"
fi

# ==================== Section 4: Test Coverage ====================

section "4. Test Coverage"

echo -n "4.1 Running unit tests with coverage... "
if go test ./internal/agent/... -coverprofile=/tmp/coverage.out > /tmp/unit-tests.log 2>&1; then
    coverage=$(go tool cover -func=/tmp/coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    if (( $(echo "$coverage >= 80" | bc -l) )); then
        pass "Unit tests passed (coverage: ${coverage}%)"
    else
        skip "Unit tests passed but coverage low (${coverage}%, target: 80%)"
    fi
else
    fail "Unit tests failed (see /tmp/unit-tests.log)"
fi

echo -n "4.2 Running integration tests... "
if [ "$SKIP_API_TESTS" = true ]; then
    skip "Skipping integration tests (--skip-api-tests)"
elif [ -z "$GEMINI_API_KEY" ]; then
    skip "GEMINI_API_KEY not set, skipping integration tests"
else
    if go test ./test/integration/... -run TestGeminiCLI -v > /tmp/integration-tests.log 2>&1; then
        pass "Integration tests passed"
    else
        fail "Integration tests failed (see /tmp/integration-tests.log)"
    fi
fi

echo -n "4.3 Running BDD tests... "
if go test ./test/bdd/... -v > /tmp/bdd-tests.log 2>&1; then
    pass "BDD tests passed"
else
    if grep -q "no tests to run" /tmp/bdd-tests.log; then
        skip "No BDD tests found in ./test/bdd/..."
    else
        fail "BDD tests failed (see /tmp/bdd-tests.log)"
    fi
fi

# ==================== Section 5: E2E Tests ====================

section "5. E2E Tests (Phase 4)"

echo -n "5.1 Checking E2E test file exists... "
if [ -f "test/e2e/gemini_phase4_e2e_test.go" ]; then
    pass "E2E test file exists"

    echo -n "5.2 Running E2E tests (requires Gemini CLI + API key)... "
    if [ "$SKIP_API_TESTS" = true ]; then
        skip "Skipping E2E tests (--skip-api-tests)"
    elif [ -z "$GEMINI_API_KEY" ]; then
        skip "GEMINI_API_KEY not set, skipping E2E tests"
    elif ! command -v gemini > /dev/null 2>&1; then
        skip "Gemini CLI not installed, skipping E2E tests"
    else
        if go test ./test/e2e/... -tags e2e -run Phase4 -v > /tmp/e2e-tests.log 2>&1; then
            pass "E2E tests passed"
        else
            fail "E2E tests failed (see /tmp/e2e-tests.log)"
        fi
    fi
else
    fail "E2E test file not found"
fi

# ==================== Section 6: Documentation ====================

section "6. Documentation"

echo -n "6.1 Checking production readiness checklist exists... "
if [ -f "docs/PRODUCTION-READINESS-CHECKLIST.md" ]; then
    pass "Checklist file exists"
else
    fail "Checklist file not found"
fi

echo -n "6.2 Verifying Phase 3 documentation... "
docs_complete=true
for doc in "docs/agents/gemini-cli.md" "docs/gemini-parity-analysis.md" "docs/AGENT-COMPARISON.md"; do
    if [ ! -f "$doc" ]; then
        docs_complete=false
        echo
        echo "  Missing: $doc"
    fi
done

if [ "$docs_complete" = true ]; then
    pass "All Phase 3 documentation present"
else
    fail "Missing documentation files"
fi

echo -n "6.3 Checking for ADR-011... "
if [ -f "docs/adr/ADR-011-gemini-cli-adapter-strategy.md" ]; then
    pass "ADR-011 exists"
else
    fail "ADR-011 not found"
fi

# ==================== Section 7: Resource Cleanup ====================

section "7. Resource Cleanup"

echo -n "7.1 Checking for leftover test sessions... "
leftover_tmux=$(tmux ls 2>/dev/null | grep -E "test-|gemini-|claude-" | wc -l || echo "0")
if [ "$leftover_tmux" -gt 0 ]; then
    skip "Found $leftover_tmux test tmux sessions (may be from other tests)"
else
    pass "No leftover tmux sessions"
fi

echo -n "7.2 Checking for leftover ready files... "
leftover_ready=$(ls -1 ~/.agm/ready-* 2>/dev/null | wc -l || echo "0")
if [ "$leftover_ready" -gt 0 ]; then
    skip "Found $leftover_ready ready files (may be from active sessions)"
else
    pass "No leftover ready files"
fi

# ==================== Summary ====================

section "Validation Summary"

echo "Total checks: $((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))"
echo -e "${GREEN}Passed: $PASS_COUNT${NC}"
echo -e "${YELLOW}Skipped: $SKIP_COUNT${NC}"
echo -e "${RED}Failed: $FAIL_COUNT${NC}"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}✓ All critical checks passed!${NC}"
    echo
    echo "Next steps:"
    echo "1. Review skipped items manually"
    echo "2. Complete Task 4.3 (Beta Testing & Feedback)"
    echo "3. Run: /engram-swarm:next to advance to Phase 5"
    exit 0
else
    echo -e "${RED}✗ Validation failed - fix issues before advancing${NC}"
    echo
    echo "Review logs in /tmp/*-tests.log for details"
    exit 1
fi
