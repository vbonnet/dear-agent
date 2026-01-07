#!/bin/bash
# Manual End-to-End Test for CSM
#
# ⚠️  DO NOT RUN IN CI/CD - This is a manual integration test
#
# This script tests CSM's core functionality including:
# - Session creation with tmux
# - UUID association
# - Session archiving
# - Archive directory scanning (fix for invisible archived sessions)
#
# Requirements:
# - tmux installed and running
# - claude command available
# - csm binary built and in PATH
#
# Usage:
#   ./tests/manual-e2e-test.sh
#
# Exit codes:
#   0 - All tests passed
#   1 - Test failed
#   2 - Prerequisites not met

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test session name (unique to avoid conflicts)
TEST_SESSION="csm-e2e-test-$$"
TEST_UUID=""
CLEANUP_DONE=false

# Cleanup function
cleanup() {
    if [ "$CLEANUP_DONE" = true ]; then
        return
    fi

    echo ""
    echo -e "${YELLOW}=== Cleanup ===${NC}"

    # Kill tmux session if it exists
    if tmux has-session -t "$TEST_SESSION" 2>/dev/null; then
        echo "Killing tmux session: $TEST_SESSION"
        tmux kill-session -t "$TEST_SESSION" 2>/dev/null || true
    fi

    # Remove archived test session
    if [ -d "$HOME/src/sessions/.archive-old-format/session-$TEST_SESSION" ]; then
        echo "Removing archived test session"
        rm -rf "$HOME/src/sessions/.archive-old-format/session-$TEST_SESSION"
    fi

    # Remove active test session
    if [ -d "$HOME/src/sessions/session-$TEST_SESSION" ]; then
        echo "Removing active test session"
        rm -rf "$HOME/src/sessions/session-$TEST_SESSION"
    fi

    CLEANUP_DONE=true
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Register cleanup on exit
trap cleanup EXIT INT TERM

# Helper functions
print_step() {
    echo ""
    echo -e "${BLUE}>>> $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

fail_test() {
    print_error "$1"
    exit 1
}

# Prerequisites check
check_prerequisites() {
    print_step "Checking prerequisites"

    if ! command -v tmux &> /dev/null; then
        fail_test "tmux not found. Please install tmux."
    fi
    print_success "tmux found: $(tmux -V)"

    if ! command -v claude &> /dev/null; then
        fail_test "claude command not found. Please ensure Claude Code CLI is installed."
    fi
    print_success "claude command found"

    if ! command -v csm &> /dev/null; then
        fail_test "csm command not found. Please build CSM first: cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager && go build -o ~/go/bin/csm ./cmd/csm"  # noqa: path-portability
    fi
    print_success "csm found: $(csm version | head -1)"

    if [ -z "$TMUX" ]; then
        print_warning "Not running inside tmux (this is OK, test will create detached session)"
    else
        print_success "Running inside tmux"
    fi
}

# Test 1: Create session and verify manifest
test_session_creation() {
    print_step "Test 1: Create tmux session and start Claude"

    # Create detached tmux session
    tmux new-session -d -s "$TEST_SESSION"
    sleep 1

    # Verify tmux session exists
    if ! tmux has-session -t "$TEST_SESSION" 2>/dev/null; then
        fail_test "Failed to create tmux session"
    fi
    print_success "Tmux session created: $TEST_SESSION"

    # Start Claude in the session
    tmux send-keys -t "$TEST_SESSION" "claude" Enter
    sleep 3

    # Check if Claude is running
    RUNNING_CMD=$(tmux list-panes -t "$TEST_SESSION" -F "#{pane_current_command}")
    if [ "$RUNNING_CMD" != "claude" ]; then
        fail_test "Claude not running in tmux session (found: $RUNNING_CMD)"
    fi
    print_success "Claude started in tmux session"

    # Check if manifest was created (by csm sync or manually)
    # For this test, we'll create it manually to simulate what happens
    if [ ! -f "$HOME/src/sessions/session-$TEST_SESSION/manifest.yaml" ]; then
        print_warning "Manifest not auto-created, creating manually for test"
        mkdir -p "$HOME/src/sessions/session-$TEST_SESSION"
        cat > "$HOME/src/sessions/session-$TEST_SESSION/manifest.yaml" <<EOF
schema_version: "2.0"
session_id: session-$TEST_SESSION
name: $TEST_SESSION
created_at: $(date -Iseconds)
updated_at: $(date -Iseconds)
lifecycle: ""
context:
    project: $HOME/src
claude: {}
tmux:
    session_name: $TEST_SESSION
EOF
    fi

    if [ ! -f "$HOME/src/sessions/session-$TEST_SESSION/manifest.yaml" ]; then
        fail_test "Manifest file not created"
    fi
    print_success "Manifest file exists"
}

# Test 2: UUID Association
test_uuid_association() {
    print_step "Test 2: Associate UUID with session"

    # Associate UUID (uses latest from history)
    OUTPUT=$(csm associate "$TEST_SESSION" 2>&1)

    if echo "$OUTPUT" | grep -q "Associated session"; then
        print_success "UUID associated successfully"
    else
        fail_test "UUID association failed: $OUTPUT"
    fi

    # Verify UUID in manifest
    if grep -q 'uuid: .\+' "$HOME/src/sessions/session-$TEST_SESSION/manifest.yaml"; then
        TEST_UUID=$(csm get-uuid "$TEST_SESSION" 2>/dev/null)
        if [ -n "$TEST_UUID" ]; then
            print_success "UUID verified in manifest: ${TEST_UUID:0:8}..."
        else
            fail_test "csm get-uuid failed for session"
        fi
    else
        fail_test "UUID not found in manifest"
    fi
}

# Test 3: Archive Session
test_session_archive() {
    print_step "Test 3: Archive session"

    # Kill tmux session first (archive requires stopped session)
    tmux kill-session -t "$TEST_SESSION"
    sleep 1

    # Archive the session
    OUTPUT=$(csm archive "$TEST_SESSION" --force 2>&1)

    if echo "$OUTPUT" | grep -q "Archived session"; then
        print_success "Session archived successfully"
    else
        fail_test "Archive command failed: $OUTPUT"
    fi

    # Verify session was moved to archive directory
    if [ -d "$HOME/src/sessions/.archive-old-format/session-$TEST_SESSION" ]; then
        print_success "Session moved to .archive-old-format/"
    else
        fail_test "Session not found in archive directory"
    fi

    # Verify lifecycle field updated
    if grep -q 'lifecycle: archived' "$HOME/src/sessions/.archive-old-format/session-$TEST_SESSION/manifest.yaml"; then
        print_success "Manifest lifecycle field set to 'archived'"
    else
        fail_test "Manifest lifecycle field not updated"
    fi
}

# Test 4: Archive Visibility (THE MAIN FIX)
test_archive_visibility() {
    print_step "Test 4: Verify archived session visibility (FIX VALIDATION)"

    # Test csm list --all shows archived session
    if csm list --all 2>/dev/null | grep -q "$TEST_SESSION"; then
        print_success "Archived session appears in 'csm list --all'"
    else
        fail_test "Archived session NOT visible in 'csm list --all' (REGRESSION!)"
    fi

    # Test csm list does NOT show archived session
    if csm list 2>/dev/null | grep -q "$TEST_SESSION"; then
        fail_test "Archived session should NOT appear in 'csm list' without --all flag"
    else
        print_success "Archived session hidden from 'csm list' (correct)"
    fi

    # Test csm get-uuid works for archived session
    ARCHIVED_UUID=$(csm get-uuid "$TEST_SESSION" 2>/dev/null)
    if [ "$ARCHIVED_UUID" = "$TEST_UUID" ]; then
        print_success "csm get-uuid works for archived session: ${ARCHIVED_UUID:0:8}..."
    else
        fail_test "csm get-uuid failed for archived session (expected: $TEST_UUID, got: $ARCHIVED_UUID)"
    fi
}

# Main test execution
main() {
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║       CSM End-to-End Manual Integration Test              ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Test session: $TEST_SESSION"
    echo ""

    check_prerequisites
    test_session_creation
    test_uuid_association
    test_session_archive
    test_archive_visibility

    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║               ALL TESTS PASSED ✓                           ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Key validations:"
    echo "  ✓ Session creation and manifest generation"
    echo "  ✓ UUID association"
    echo "  ✓ Session archival"
    echo "  ✓ Archived sessions visible in 'csm list --all'"
    echo "  ✓ Archived sessions accessible via 'csm get-uuid'"
    echo ""

    exit 0
}

# Run tests
main
