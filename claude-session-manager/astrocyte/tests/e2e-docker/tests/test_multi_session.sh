#!/bin/bash
# Test: Multi-session monitoring
# Verifies astrocyte monitors multiple sessions and only recovers stuck ones

set -euo pipefail

TEST_NAME="multi_session_monitoring"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
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

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

# Header
echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║  Test: Multi-Session Monitoring                           ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Setup
log_info "Setting up test environment..."
/tests/scripts/setup-test-env.sh

# Create 3 sessions: 1 stuck, 2 normal
log_info "Creating test sessions..."
/tests/scripts/create-stuck-session.sh "session-stuck" "stuck-mustering.txt"
/tests/scripts/create-stuck-session.sh "session-normal-1" "normal-session.txt"
/tests/scripts/create-stuck-session.sh "session-normal-2" "normal-session.txt"

# Run astrocyte for 3 check cycles
log_info "Running astrocyte daemon (15 seconds)..."
/tests/scripts/run-astrocyte-test.sh 15

# Verify results
log_info "Verifying recovery..."

INCIDENTS_LOG="/home/testuser/.csm/astrocyte/incidents.jsonl"
PASSED=0
FAILED=0

# Test 1: Stuck session was recovered
echo ""
echo "=== Test 1: Stuck Session Recovered ==="
if grep -q "\"session_name\": \"session-stuck\"" "$INCIDENTS_LOG"; then
    log_pass "Stuck session was detected and logged"
    ((PASSED++))
else
    log_fail "Stuck session was NOT detected"
    ((FAILED++))
fi

# Test 2: Normal sessions were NOT flagged
echo ""
echo "=== Test 2: Normal Sessions Not Flagged ==="
if grep -q "\"session_name\": \"session-normal" "$INCIDENTS_LOG"; then
    log_fail "Normal sessions were incorrectly flagged as stuck"
    ((FAILED++))
else
    log_pass "Normal sessions were correctly ignored"
    ((PASSED++))
fi

# Test 3: Only 1 incident logged
echo ""
echo "=== Test 3: Incident Count ==="
INCIDENT_COUNT=$(grep -c "session_name" "$INCIDENTS_LOG" || echo "0")
if [ "$INCIDENT_COUNT" -ge 1 ]; then
    log_pass "Found $INCIDENT_COUNT incident(s) - expected at least 1"
    ((PASSED++))
else
    log_fail "Expected at least 1 incident, found $INCIDENT_COUNT"
    ((FAILED++))
fi

# Summary
echo ""
echo "=== Test Summary ==="
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo ""

if [ "$FAILED" -eq 0 ]; then
    log_pass "Multi-session monitoring test PASSED"
    exit 0
else
    log_fail "Multi-session monitoring test FAILED"
    exit 1
fi
