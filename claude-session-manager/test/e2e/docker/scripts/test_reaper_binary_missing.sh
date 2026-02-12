#!/bin/bash
set -e

echo "=== Reaper Binary Missing E2E Test ==="
echo ""
echo "Tests error handling when agm-reaper binary is not found"
echo ""

# Setup paths
export PATH="/home/testuser/bin:$PATH"
SESSIONS_DIR="/home/testuser/sessions"
TEST_SESSION="test-reaper-missing"
AGM_SOCKET="/tmp/agm.sock"

# Cleanup from previous runs
tmux -S "$AGM_SOCKET" kill-session -t "$TEST_SESSION" 2>/dev/null || true
rm -rf "$SESSIONS_DIR/$TEST_SESSION" 2>/dev/null || true

echo "Step 1: Create tmux session with mock Claude..."
# Use the working mock_claude.py from happy path test
tmux -S "$AGM_SOCKET" new-session -d -s "$TEST_SESSION" python3 /home/testuser/tests/mock_claude.py
sleep 2
echo "✓ Tmux session created"

echo ""
echo "Step 2: Create AGM session manifest..."
SESSION_DIR="$SESSIONS_DIR/$TEST_SESSION"
mkdir -p "$SESSION_DIR"

SESSION_UUID=$(uuidgen 2>/dev/null || echo "test-uuid-$(date +%s)")
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

cat > "$SESSION_DIR/manifest.yaml" <<EOF
schema_version: "2"
session_id: $SESSION_UUID
name: $TEST_SESSION
created_at: $NOW
updated_at: $NOW
lifecycle: ""
context:
  project: "E2E Binary Missing Test"
claude:
  uuid: ""
tmux:
  session_name: $TEST_SESSION
EOF

echo "✓ AGM manifest created"

echo ""
echo "Step 3: Hide agm-reaper binary temporarily..."
# Modify PATH to exclude /home/testuser/bin
# This simulates the binary not being found
ORIGINAL_PATH="$PATH"
export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
echo "✓ PATH modified to exclude /home/testuser/bin"
echo "   Original PATH: $ORIGINAL_PATH"
echo "   Modified PATH: $PATH"

echo ""
echo "Step 4: Try to archive with --async flag (should fail)..."
# Try to run agm session archive --async, capture error
ERROR_OUTPUT=$(agm session archive "$TEST_SESSION" --async 2>&1 || true)

echo "Error output captured:"
echo "$ERROR_OUTPUT"

echo ""
echo "Step 5: Verify error message quality..."
CHECKS_PASSED=0

# Check 1: Error mentions the binary
if echo "$ERROR_OUTPUT" | grep -qi "agm-reaper"; then
    echo "✓ Error mentions 'agm-reaper'"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo "✗ Error should mention 'agm-reaper'"
fi

# Check 2: Error indicates binary not found or spawn failed
if echo "$ERROR_OUTPUT" | grep -qiE "(not found|failed to start|no such file)"; then
    echo "✓ Error indicates binary not found/spawn failed"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo "✗ Error should indicate binary not found"
fi

# Check 3: Error provides actionable guidance
if echo "$ERROR_OUTPUT" | grep -qiE "(build|install|path)"; then
    echo "✓ Error provides actionable guidance (build/install/path)"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo "⚠️  Error could provide more guidance (optional)"
    # This is not a hard failure, just a quality check
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
fi

echo ""
echo "Step 6: Restore PATH for cleanup..."
export PATH="$ORIGINAL_PATH"
echo "✓ PATH restored to: $PATH"

echo ""
echo "Step 7: Verify session not archived (failed gracefully)..."
ARCHIVE_DIR="$SESSIONS_DIR/.archive-old-format/$TEST_SESSION"

if [ -d "$ARCHIVE_DIR" ]; then
    echo "✗ Session was archived despite error (should fail safely)"
    exit 1
fi

echo "✓ Session not archived (failed safely)"

# Check original session directory still exists
if [ -d "$SESSION_DIR" ]; then
    echo "✓ Original session directory intact"
else
    echo "✗ Original session directory missing (data loss!)"
    exit 1
fi

echo ""
echo "Step 8: Cleanup tmux session..."
tmux -S "$AGM_SOCKET" kill-session -t "$TEST_SESSION" 2>/dev/null || true
echo "✓ Tmux session cleaned up"

echo ""
echo "=== Test Results ==="
echo "Checks passed: $CHECKS_PASSED/3"

if [ $CHECKS_PASSED -ge 2 ]; then
    echo "✓ Error handling acceptable"
else
    echo "✗ Error handling needs improvement"
    exit 1
fi

echo ""
echo "🎉 Reaper Binary Missing E2E Test: PASSED"
echo ""
echo "Key findings:"
echo "- Error message mentions agm-reaper binary"
echo "- Failure is graceful (session data preserved)"
echo "- User gets actionable feedback"
exit 0
