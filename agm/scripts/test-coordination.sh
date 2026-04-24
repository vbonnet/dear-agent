#!/usr/bin/env bash
# AGM Coordination End-to-End Test Script
# Tests complete installation and message delivery flow

set -euo pipefail

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Test configuration
TEST_SESSION_1="agm-test-sender-$$"
TEST_SESSION_2="agm-test-receiver-$$"
TEST_MESSAGE="Hello from AGM coordination test at $(date)"
TIMEOUT=60  # seconds to wait for delivery

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
}

test_assert() {
    local description=$1
    local command=$2

    ((TESTS_RUN++))

    if eval "$command" &>/dev/null; then
        log_success "$description"
        ((TESTS_PASSED++))
        return 0
    else
        log_fail "$description"
        ((TESTS_FAILED++))
        return 1
    fi
}

cleanup() {
    log_info "Cleaning up test sessions..."

    # Kill test sessions
    agm kill "$TEST_SESSION_1" --force &>/dev/null || true
    agm kill "$TEST_SESSION_2" --force &>/dev/null || true

    # Archive test sessions
    agm archive "$TEST_SESSION_1" &>/dev/null || true
    agm archive "$TEST_SESSION_2" &>/dev/null || true
}

trap cleanup EXIT

print_header() {
    echo -e "${BLUE}"
    cat <<'EOF'
╔═══════════════════════════════════════════════════════════════════════╗
║                                                                       ║
║   AGM Coordination End-to-End Test Suite                             ║
║   Phase 3 Task 3.1: Installation & Migration Testing                 ║
║                                                                       ║
╚═══════════════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"
}

test_prerequisites() {
    log_info "Testing prerequisites..."

    test_assert "AGM binary is available" "command -v agm"
    test_assert "agm-daemon binary is available" "command -v agm-daemon"
    test_assert "Tmux is installed" "command -v tmux"
    test_assert "SQLite is installed" "command -v sqlite3"
}

test_hooks() {
    log_info "Testing hook installation..."

    test_assert "posttool hook exists" \
        "[ -f ~/.claude/hooks/posttool-agm-state-notify ]"

    test_assert "posttool hook is executable" \
        "[ -x ~/.claude/hooks/posttool-agm-state-notify ]"

    test_assert "session-start hook exists" \
        "[ -f ~/.claude/hooks/session-start/agm-state-ready ]"

    test_assert "session-start hook is executable" \
        "[ -x ~/.claude/hooks/session-start/agm-state-ready ]"
}

test_daemon() {
    log_info "Testing daemon status..."

    test_assert "Daemon systemd service exists" \
        "systemctl --user list-unit-files | grep -q agm-daemon"

    test_assert "Daemon is running" \
        "systemctl --user is-active agm-daemon"

    test_assert "Daemon PID file exists" \
        "[ -f ~/.agm/daemon.pid ]"

    test_assert "agm daemon status works" \
        "agm daemon status"
}

test_queue_database() {
    log_info "Testing queue database..."

    test_assert "Queue database exists" \
        "[ -f ~/.agm/queue.db ]"

    test_assert "Queue database is readable" \
        "sqlite3 ~/.agm/queue.db 'SELECT COUNT(*) FROM message_queue;'"

    test_assert "Queue database has correct schema" \
        "sqlite3 ~/.agm/queue.db '.schema' | grep -q 'CREATE TABLE.*message_queue'"

    test_assert "Queue database has indexes" \
        "sqlite3 ~/.agm/queue.db '.schema' | grep -q 'CREATE INDEX.*idx_pending'"

    test_assert "Queue database WAL mode enabled" \
        "[ \$(sqlite3 ~/.agm/queue.db 'PRAGMA journal_mode;') = 'wal' ]"
}

test_session_creation() {
    log_info "Testing session creation with state tracking..."

    # Create test sessions
    log_info "Creating test sessions..."

    # Session 1 (sender)
    if agm new "$TEST_SESSION_1" --project ~/tmp --non-interactive 2>/dev/null; then
        log_success "Created sender session: $TEST_SESSION_1"
        ((TESTS_PASSED++))
    else
        log_fail "Failed to create sender session"
        ((TESTS_FAILED++))
    fi
    ((TESTS_RUN++))

    # Session 2 (receiver)
    if agm new "$TEST_SESSION_2" --project ~/tmp --non-interactive 2>/dev/null; then
        log_success "Created receiver session: $TEST_SESSION_2"
        ((TESTS_PASSED++))
    else
        log_fail "Failed to create receiver session"
        ((TESTS_FAILED++))
    fi
    ((TESTS_RUN++))

    # Wait for sessions to initialize
    sleep 2

    test_assert "Sender session manifest exists" \
        "[ -f ~/.agm/sessions/$TEST_SESSION_1/manifest.json ]"

    test_assert "Receiver session manifest exists" \
        "[ -f ~/.agm/sessions/$TEST_SESSION_2/manifest.json ]"

    test_assert "Sender manifest has state field" \
        "jq -e '.state' ~/.agm/sessions/$TEST_SESSION_1/manifest.json"

    test_assert "Receiver manifest has state field" \
        "jq -e '.state' ~/.agm/sessions/$TEST_SESSION_2/manifest.json"
}

test_message_delivery() {
    log_info "Testing message delivery..."

    # Get initial queue state
    local initial_pending=$(sqlite3 ~/.agm/queue.db "SELECT COUNT(*) FROM message_queue WHERE status='pending';" 2>/dev/null || echo "0")

    # Send message
    log_info "Sending test message..."
    if agm send "$TEST_SESSION_1" "$TEST_SESSION_2" "$TEST_MESSAGE" 2>/dev/null; then
        log_success "Message enqueued"
        ((TESTS_PASSED++))
    else
        log_fail "Failed to enqueue message"
        ((TESTS_FAILED++))
    fi
    ((TESTS_RUN++))

    # Verify message in queue
    test_assert "Message added to queue" \
        "[ \$(sqlite3 ~/.agm/queue.db \"SELECT COUNT(*) FROM message_queue WHERE status='pending';\") -gt $initial_pending ]"

    # Wait for delivery (daemon polls every 30s)
    log_info "Waiting up to ${TIMEOUT}s for message delivery..."
    local waited=0
    local delivered=false

    while [ $waited -lt $TIMEOUT ]; do
        local pending=$(sqlite3 ~/.agm/queue.db "SELECT COUNT(*) FROM message_queue WHERE status='pending';" 2>/dev/null || echo "0")

        if [ "$pending" -lt "$(($initial_pending + 1))" ]; then
            delivered=true
            break
        fi

        sleep 5
        ((waited+=5))
        echo -n "."
    done
    echo

    ((TESTS_RUN++))
    if [ "$delivered" = true ]; then
        log_success "Message delivered within ${waited}s"
        ((TESTS_PASSED++))
    else
        log_fail "Message not delivered after ${TIMEOUT}s"
        ((TESTS_FAILED++))
    fi
}

test_state_transitions() {
    log_info "Testing state transitions..."

    # Get current state
    local state=$(agm session state get "$TEST_SESSION_2" 2>/dev/null || echo "UNKNOWN")

    test_assert "Can query session state" \
        "[ \"$state\" != \"UNKNOWN\" ]"

    # Test manual state setting
    log_info "Testing manual state updates..."
    if agm session state set "$TEST_SESSION_2" READY 2>/dev/null; then
        log_success "Set state to READY"
        ((TESTS_PASSED++))
    else
        log_fail "Failed to set state"
        ((TESTS_FAILED++))
    fi
    ((TESTS_RUN++))

    test_assert "State persisted to manifest" \
        "[ \$(jq -r '.state' ~/.agm/sessions/$TEST_SESSION_2/manifest.json) = 'READY' ]"
}

test_lingering() {
    log_info "Testing user lingering..."

    test_assert "User lingering is enabled" \
        "loginctl show-user \$USER | grep -q 'Linger=yes'"
}

print_summary() {
    echo
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}Test Summary${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════════════${NC}"
    echo
    echo "  Total Tests:  $TESTS_RUN"
    echo -e "  ${GREEN}Passed:       $TESTS_PASSED${NC}"
    echo -e "  ${RED}Failed:       $TESTS_FAILED${NC}"
    echo

    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "${GREEN}✓ All tests passed!${NC}"
        echo
        echo "Your AGM coordination installation is working correctly."
        return 0
    else
        echo -e "${RED}✗ Some tests failed${NC}"
        echo
        echo "Please review the failures above and check:"
        echo "  - Daemon logs: journalctl --user -u agm-daemon -f"
        echo "  - Queue status: agm daemon status"
        echo "  - Health check: agm doctor --validate"
        return 1
    fi
}

main() {
    print_header

    log_info "Starting end-to-end tests..."
    echo

    # Run test suites
    test_prerequisites
    echo

    test_hooks
    echo

    test_daemon
    echo

    test_queue_database
    echo

    test_session_creation
    echo

    test_message_delivery
    echo

    test_state_transitions
    echo

    test_lingering
    echo

    # Print summary
    print_summary
}

# Run main if executed directly
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi
