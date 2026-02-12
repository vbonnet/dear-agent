#!/bin/bash
# Test: Zero-token waiting detection and recovery
# Verifies astrocyte detects "↓ 0 tokens" pattern and sends ESC + diagnosis

set -euo pipefail

TEST_NAME="zero_token_recovery"
SESSION_NAME="test-zero-token"
FIXTURE="stuck-zero-token.txt"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[TEST]${NC} $*"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
}

# Header
echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║  Test: Zero-Token Waiting Detection                       ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Setup
log_info "Setting up test environment..."
/tests/scripts/setup-test-env.sh

# Create stuck session
log_info "Creating stuck session with zero-token pattern..."
/tests/scripts/create-stuck-session.sh "$SESSION_NAME" "$FIXTURE"

# Run astrocyte for 3 check cycles
log_info "Running astrocyte daemon (15 seconds)..."
/tests/scripts/run-astrocyte-test.sh 15

# Verify recovery
log_info "Verifying recovery..."
if /tests/scripts/verify-recovery.sh "$SESSION_NAME" "stuck_zero_token_waiting"; then
    log_pass "Zero-token detection test PASSED"

    # Additional check: Verify diagnosis prompt was sent
    CSM_LOG="/home/testuser/.agm/astrocyte/logs/csm-mock.log"
    if [ -f "$CSM_LOG" ] && grep -q "csm send.*$SESSION_NAME" "$CSM_LOG"; then
        log_pass "Diagnosis prompt was sent via csm send"
    else
        log_fail "Diagnosis prompt was NOT sent"
        exit 1
    fi

    exit 0
else
    log_fail "Zero-token detection test FAILED"
    exit 1
fi
