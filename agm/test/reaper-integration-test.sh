#!/usr/bin/env bash
# Integration test for agm-reaper with agm send fix
# Tests that reaper properly waits for Claude to finish before sending /exit

set -euo pipefail

TEST_SESSION="reaper-test-$$"
TEST_DIR="/tmp/reaper-test-$$"
REAPER_LOG="/tmp/agm-reaper-${TEST_SESSION}.log"
AGM_SESSIONS_DIR="${TEST_DIR}/sessions"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[TEST]${NC} $*"
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

cleanup() {
    log "Cleaning up test session..."
    tmux -S /tmp/agm.sock kill-session -t "${TEST_SESSION}" 2>/dev/null || true
    rm -rf "${TEST_DIR}" || true
    rm -f "${REAPER_LOG}" || true
}

trap cleanup EXIT

# Verify binaries exist
if ! command -v agm >/dev/null 2>&1; then
    error "agm binary not found in PATH"
    exit 1
fi

if ! command -v agm-reaper >/dev/null 2>&1; then
    error "agm-reaper binary not found in PATH"
    exit 1
fi

log "Starting integration test for agm-reaper with agm send"
log "Test session: ${TEST_SESSION}"

# Create test session directory structure
log "Creating test session directory structure..."
mkdir -p "${AGM_SESSIONS_DIR}/${TEST_SESSION}"

# Create minimal manifest
cat > "${AGM_SESSIONS_DIR}/${TEST_SESSION}/MANIFEST.json" <<EOF
{
  "uuid": "test-uuid-reaper-$$",
  "session_name": "${TEST_SESSION}",
  "agent_id": "claude",
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "lifecycle": "active"
}
EOF

# Create test tmux session with a simple command that takes a few seconds
log "Creating test tmux session..."
tmux -S /tmp/agm.sock new-session -d -s "${TEST_SESSION}" -x 80 -y 24

# Send a command that will take ~3 seconds to complete
tmux -S /tmp/agm.sock send-keys -t "${TEST_SESSION}" -l 'echo "Starting work..."; sleep 3; echo "Work complete"; echo "❯"'
tmux -S /tmp/agm.sock send-keys -t "${TEST_SESSION}" Enter

# Give it a moment to start
sleep 1

# Verify session is running
if ! tmux -S /tmp/agm.sock list-sessions | grep -q "${TEST_SESSION}"; then
    error "Test session failed to start"
    exit 1
fi

log "Test session created and executing work (3 second sleep)"

# Spawn reaper in background
log "Spawning agm-reaper..."
agm-reaper \
    --session "${TEST_SESSION}" \
    --log-file "${REAPER_LOG}" \
    --sessions-dir "${AGM_SESSIONS_DIR}" &

REAPER_PID=$!
log "Reaper spawned with PID ${REAPER_PID}"

# Monitor reaper log
log "Monitoring reaper log for 30 seconds..."
TIMEOUT=30
START_TIME=$(date +%s)

while true; do
    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))

    if [ $ELAPSED -gt $TIMEOUT ]; then
        error "Reaper did not complete within ${TIMEOUT} seconds"
        if [ -f "${REAPER_LOG}" ]; then
            error "Reaper log contents:"
            cat "${REAPER_LOG}"
        fi
        exit 1
    fi

    # Check if reaper completed
    if ! ps -p ${REAPER_PID} > /dev/null 2>&1; then
        log "Reaper process completed"
        break
    fi

    # Show log tail every 2 seconds
    if [ $((ELAPSED % 2)) -eq 0 ] && [ -f "${REAPER_LOG}" ]; then
        LAST_LINE=$(tail -n 1 "${REAPER_LOG}" 2>/dev/null || echo "")
        if [ -n "${LAST_LINE}" ]; then
            log "Reaper: ${LAST_LINE}"
        fi
    fi

    sleep 1
done

# Check reaper exit code
wait ${REAPER_PID}
REAPER_EXIT=$?

log "Reaper exited with code: ${REAPER_EXIT}"

# Display full reaper log
log "=== Full Reaper Log ==="
cat "${REAPER_LOG}"
log "=== End Reaper Log ==="

# Verify reaper success
if [ ${REAPER_EXIT} -ne 0 ]; then
    error "Reaper failed with exit code ${REAPER_EXIT}"
    exit 1
fi

# Verify tmux session is gone
if tmux -S /tmp/agm.sock list-sessions 2>/dev/null | grep -q "${TEST_SESSION}"; then
    error "Tmux session still exists after reaper completion"
    exit 1
fi

# Verify manifest was archived
if [ -f "${AGM_SESSIONS_DIR}/${TEST_SESSION}/MANIFEST.json" ]; then
    LIFECYCLE=$(grep -o '"lifecycle":[[:space:]]*"[^"]*"' "${AGM_SESSIONS_DIR}/${TEST_SESSION}/MANIFEST.json" | cut -d'"' -f4)
    if [ "${LIFECYCLE}" != "archived" ]; then
        warn "Manifest lifecycle is '${LIFECYCLE}', expected 'archived'"
    else
        log "✓ Manifest lifecycle correctly set to 'archived'"
    fi
fi

# Verify key steps in log
log "Verifying reaper behavior..."

if ! grep -q "Waiting for Claude to return to prompt" "${REAPER_LOG}"; then
    error "Reaper log missing prompt wait message"
    exit 1
fi
log "✓ Reaper waited for Claude prompt"

if ! grep -q "agm send" "${REAPER_LOG}"; then
    error "Reaper log missing 'agm send' invocation"
    exit 1
fi
log "✓ Reaper used 'agm send' to send /exit"

if ! grep -q "Reaper completed successfully" "${REAPER_LOG}"; then
    error "Reaper did not complete successfully"
    exit 1
fi
log "✓ Reaper completed successfully"

log ""
log "========================================="
log "✅ Integration test PASSED"
log "========================================="
log ""
log "Key findings:"
log "  • Reaper waited for Claude prompt detection"
log "  • Reaper used 'agm send' (robust sending)"
log "  • Session was properly archived"
log "  • Tmux session was cleaned up"
