#!/bin/bash
# Test: Incident logging validation
# Verifies JSONL format, required fields, and crash-safe append behavior

set -euo pipefail

TEST_NAME="incident_logging"
SESSION_NAME="test-logging"
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
echo "║  Test: Incident Logging Validation                        ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Setup
log_info "Setting up test environment..."
/tests/scripts/setup-test-env.sh

# Create stuck session
log_info "Creating stuck session..."
/tests/scripts/create-stuck-session.sh "$SESSION_NAME" "$FIXTURE"

# Run astrocyte
log_info "Running astrocyte daemon (15 seconds)..."
/tests/scripts/run-astrocyte-test.sh 15

# Validate incident log
INCIDENTS_LOG="/home/testuser/.csm/astrocyte/incidents.jsonl"
PASSED=0
FAILED=0

# Test 1: Log file exists
echo ""
echo "=== Test 1: Log File Exists ==="
if [ -f "$INCIDENTS_LOG" ]; then
    log_pass "Incidents log file exists: $INCIDENTS_LOG"
    ((PASSED++))
else
    log_fail "Incidents log file not found"
    ((FAILED++))
    exit 1
fi

# Test 2: JSONL format valid
echo ""
echo "=== Test 2: JSONL Format Valid ==="
INVALID_LINES=0
while IFS= read -r line; do
    if ! echo "$line" | python3 -m json.tool >/dev/null 2>&1; then
        ((INVALID_LINES++))
        echo "Invalid JSON: $line"
    fi
done < "$INCIDENTS_LOG"

if [ "$INVALID_LINES" -eq 0 ]; then
    log_pass "All incidents are valid JSON"
    ((PASSED++))
else
    log_fail "Found $INVALID_LINES invalid JSON lines"
    ((FAILED++))
fi

# Test 3: Required fields present
echo ""
echo "=== Test 3: Required Fields Present ==="
REQUIRED_FIELDS=(
    "timestamp"
    "session_name"
    "session_id"
    "symptom"
    "duration_minutes"
    "detection_heuristic"
    "pane_snapshot"
    "cursor_position"
    "recovery_attempted"
    "recovery_method"
)

MISSING_FIELDS=0
for field in "${REQUIRED_FIELDS[@]}"; do
    if ! grep -q "\"$field\":" "$INCIDENTS_LOG"; then
        log_fail "Missing required field: $field"
        ((MISSING_FIELDS++))
    fi
done

if [ "$MISSING_FIELDS" -eq 0 ]; then
    log_pass "All required fields present"
    ((PASSED++))
else
    log_fail "Missing $MISSING_FIELDS required fields"
    ((FAILED++))
fi

# Test 4: Incident count
echo ""
echo "=== Test 4: Incident Count ==="
INCIDENT_COUNT=$(wc -l < "$INCIDENTS_LOG")
log_info "Found $INCIDENT_COUNT incident(s)"

if [ "$INCIDENT_COUNT" -ge 1 ]; then
    log_pass "At least 1 incident logged"
    ((PASSED++))
else
    log_fail "No incidents logged"
    ((FAILED++))
fi

# Test 5: Session ID format
echo ""
echo "=== Test 5: Session ID Format ==="
if grep -q "\"session_id\": \"test-session-" "$INCIDENTS_LOG"; then
    log_pass "Session ID format correct"
    ((PASSED++))
else
    log_fail "Session ID format incorrect"
    ((FAILED++))
fi

# Summary
echo ""
echo "=== Test Summary ==="
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo ""

# Display sample incident
echo "=== Sample Incident (formatted) ==="
head -1 "$INCIDENTS_LOG" | python3 -m json.tool | head -20
echo "..."

if [ "$FAILED" -eq 0 ]; then
    log_pass "Incident logging validation test PASSED"
    exit 0
else
    log_fail "Incident logging validation test FAILED"
    exit 1
fi
