#!/bin/bash
# Verify astrocyte recovery results
# Usage: verify-recovery.sh <session-name> <expected-symptom>

set -euo pipefail

SESSION_NAME="$1"
EXPECTED_SYMPTOM="${2:-}"

INCIDENTS_LOG="/home/testuser/.agm/astrocyte/incidents.jsonl"
CSM_MOCK_LOG="/home/testuser/.agm/astrocyte/logs/csm-mock.log"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[✓]${NC} $*"
}

log_error() {
    echo -e "${RED}[✗]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[!]${NC} $*"
}

PASSED=0
FAILED=0

# Test: Incident logged
test_incident_logged() {
    echo ""
    echo "=== Test: Incident Logged ==="

    if [ ! -f "$INCIDENTS_LOG" ]; then
        log_error "Incidents log file not found: $INCIDENTS_LOG"
        ((FAILED++))
        return 1
    fi

    # Check if session appears in log
    if grep -q "\"session_name\": \"$SESSION_NAME\"" "$INCIDENTS_LOG"; then
        log_info "Incident logged for session: $SESSION_NAME"
        ((PASSED++))
    else
        log_error "No incident logged for session: $SESSION_NAME"
        ((FAILED++))
        return 1
    fi

    # Verify symptom if provided
    if [ -n "$EXPECTED_SYMPTOM" ]; then
        if grep -q "\"symptom\": \"$EXPECTED_SYMPTOM\"" "$INCIDENTS_LOG"; then
            log_info "Correct symptom detected: $EXPECTED_SYMPTOM"
            ((PASSED++))
        else
            log_error "Expected symptom not found: $EXPECTED_SYMPTOM"
            echo "Logged incidents:"
            grep "session_name.*$SESSION_NAME" "$INCIDENTS_LOG" || true
            ((FAILED++))
            return 1
        fi
    fi
}

# Test: Recovery attempted
test_recovery_attempted() {
    echo ""
    echo "=== Test: Recovery Attempted ==="

    if [ ! -f "$INCIDENTS_LOG" ]; then
        log_error "Cannot verify recovery - incidents log missing"
        ((FAILED++))
        return 1
    fi

    # Check recovery_attempted field
    if grep "\"session_name\": \"$SESSION_NAME\"" "$INCIDENTS_LOG" | grep -q "\"recovery_attempted\": true"; then
        log_info "Recovery was attempted"
        ((PASSED++))
    else
        log_error "Recovery was not attempted"
        ((FAILED++))
        return 1
    fi
}

# Test: JSONL format valid
test_jsonl_format() {
    echo ""
    echo "=== Test: JSONL Format Valid ==="

    if [ ! -f "$INCIDENTS_LOG" ]; then
        log_error "Incidents log file not found"
        ((FAILED++))
        return 1
    fi

    # Validate each line is valid JSON
    local INVALID_LINES=0
    while IFS= read -r line; do
        if ! echo "$line" | python3 -m json.tool >/dev/null 2>&1; then
            ((INVALID_LINES++))
        fi
    done < "$INCIDENTS_LOG"

    if [ "$INVALID_LINES" -eq 0 ]; then
        log_info "All incidents are valid JSON"
        ((PASSED++))
    else
        log_error "Found $INVALID_LINES invalid JSON lines"
        ((FAILED++))
        return 1
    fi
}

# Test: AGM commands called (if mock log exists)
test_csm_commands() {
    echo ""
    echo "=== Test: AGM Commands Called ==="

    if [ ! -f "$CSM_MOCK_LOG" ]; then
        log_warn "AGM mock log not found - skipping test"
        return 0
    fi

    # Check if csm send or csm reject was called
    if grep -q "csm send\|csm reject" "$CSM_MOCK_LOG"; then
        log_info "AGM commands were invoked"
        ((PASSED++))

        # Show which commands were called
        echo "Commands called:"
        grep "csm send\|csm reject" "$CSM_MOCK_LOG" | sed 's/^/  /'
    else
        log_warn "No AGM commands were called"
    fi
}

# Run all tests
main() {
    echo ""
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║       Astrocyte Recovery Verification                     ║"
    echo "║       Session: $SESSION_NAME"
    echo "╚════════════════════════════════════════════════════════════╝"

    test_incident_logged
    test_recovery_attempted
    test_jsonl_format
    test_csm_commands

    # Summary
    echo ""
    echo "=== Test Summary ==="
    echo "Passed: $PASSED"
    echo "Failed: $FAILED"
    echo ""

    if [ "$FAILED" -eq 0 ]; then
        log_info "All verifications passed ✓"
        exit 0
    else
        log_error "Some verifications failed ✗"
        exit 1
    fi
}

main
