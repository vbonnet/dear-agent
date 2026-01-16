#!/usr/bin/env bash
set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helpers
# shellcheck source=./test-helpers.sh
. "$SCRIPT_DIR/test-helpers.sh"

echo "═══════════════════════════════════════════"
echo "  claude-session-manager E2E Installation Test Suite"
echo "═══════════════════════════════════════════"
echo ""

# Track test results
TESTS_PASSED=0
TESTS_FAILED=0

# Test 1: Binary verification
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Running binary verification tests..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if bash "$SCRIPT_DIR/verify-binary.sh"; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "Binary verification failed"
    exit 1  # Fail fast
fi
echo ""

# Summary
echo "═══════════════════════════════════════════"
echo "  Test Results Summary"
echo "═══════════════════════════════════════════"
log_success "Tests passed: $TESTS_PASSED"
if [ $TESTS_FAILED -gt 0 ]; then
    log_error "Tests failed: $TESTS_FAILED"
    exit 1
fi

echo ""
log_success "All E2E installation tests passed! ✨"
echo ""
exit 0
