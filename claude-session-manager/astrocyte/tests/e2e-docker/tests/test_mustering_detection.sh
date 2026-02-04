#!/bin/bash
# Test: Mustering timeout detection and recovery
# Verifies astrocyte detects "✻ Mustering..." pattern and sends ESC

set -euo pipefail

TEST_NAME="mustering_detection"
SESSION_NAME="test-mustering"
FIXTURE="stuck-mustering.txt"

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
echo "║  Test: Mustering Timeout Detection                        ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Setup
log_info "Setting up test environment..."
/tests/scripts/setup-test-env.sh

# Create stuck session
log_info "Creating stuck session with mustering pattern..."
/tests/scripts/create-stuck-session.sh "$SESSION_NAME" "$FIXTURE"

# Run astrocyte for 2 check cycles (10 seconds with 5s interval)
# With 1-minute threshold, session should be detected as stuck
log_info "Running astrocyte daemon (15 seconds, 3 check cycles)..."
/tests/scripts/run-astrocyte-test.sh 15

# Verify recovery
log_info "Verifying recovery..."
if /tests/scripts/verify-recovery.sh "$SESSION_NAME" "stuck_mustering"; then
    log_pass "Mustering detection test PASSED"
    exit 0
else
    log_fail "Mustering detection test FAILED"
    exit 1
fi
