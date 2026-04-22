#!/bin/bash
set -e

echo "=== Reaper Prompt Timeout E2E Test ==="
echo ""
echo "Tests fallback behavior when Claude never returns to prompt"
echo ""
echo "⚠️  NOTE: This test takes ~150s to run (90s timeout + 60s fallback)"
echo "It's disabled by default in run-reaper-tests.sh for faster CI."
echo "Enable for thorough validation of timeout/fallback logic."
echo ""

# Setup paths
export PATH="/home/testuser/bin:$PATH"
SESSIONS_DIR="/home/testuser/sessions"
TEST_SESSION="test-reaper-timeout"
REAPER_LOG="/tmp/agm-reaper-${TEST_SESSION}.log"
AGM_SOCKET="/tmp/agm.sock"

# Cleanup from previous runs
tmux -S "$AGM_SOCKET" kill-session -t "$TEST_SESSION" 2>/dev/null || true
rm -rf "$SESSIONS_DIR/$TEST_SESSION" 2>/dev/null || true
rm -f "$REAPER_LOG" 2>/dev/null || true

echo "Step 1: Create tmux session with stuck Claude (never shows prompt)..."
# Create a Python script that simulates stuck Claude - outputs text but never shows prompt
cat > /tmp/mock_claude_stuck.py <<'PYTHON'
#!/usr/bin/env python3
"""
Mock Claude that never shows prompt (simulates stuck/busy state).
Used to test reaper's timeout and fallback behavior.
"""

import sys
import time

def main():
    print("Mock Claude Code v1.0 (Stuck Simulation)")
    print("Processing request... (will never finish)")
    print()

    # Never show prompt, just keep outputting dots to simulate activity
    try:
        while True:
            sys.stdout.write(".")
            sys.stdout.flush()
            time.sleep(5)
    except KeyboardInterrupt:
        print()
        print("Interrupted")
        sys.exit(1)

if __name__ == "__main__":
    main()
PYTHON

chmod +x /tmp/mock_claude_stuck.py

# Run stuck Python script directly in tmux session
tmux -S "$AGM_SOCKET" new-session -d -s "$TEST_SESSION" python3 /tmp/mock_claude_stuck.py
sleep 2  # Wait for mock Claude to start
echo "✓ Tmux session created with stuck Claude (socket: $AGM_SOCKET)"

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
  project: "E2E Timeout Test"
claude:
  uuid: ""
tmux:
  session_name: $TEST_SESSION
EOF

echo "✓ AGM manifest created: $SESSION_DIR/manifest.yaml"

echo ""
echo "Step 3: Verify stuck Claude is running (no prompt expected)..."
sleep 1
CAPTURE=$(tmux -S "$AGM_SOCKET" capture-pane -t "$TEST_SESSION" -p)
if echo "$CAPTURE" | grep -q "Processing request"; then
    echo "✓ Stuck Claude is running (outputting dots)"
else
    echo "✗ Stuck Claude not detected"
    echo "Captured output:"
    echo "$CAPTURE"
    exit 1
fi

echo ""
echo "Step 4: Spawn async archive with agm-reaper..."
echo "Expected: Reaper will timeout after 90s, fall back to 60s wait"
echo "Total expected time: ~150s (90s timeout + 60s fallback)"

/home/testuser/bin/agm-reaper \
    --session "$TEST_SESSION" \
    --log-file "$REAPER_LOG" \
    --sessions-dir "$SESSIONS_DIR" &

REAPER_PID=$!
echo "✓ Reaper spawned with PID: $REAPER_PID"

echo ""
echo "Step 5: Monitor reaper log for timeout fallback (max 180s)..."
START_TIME=$(date +%s)
TIMEOUT=180

# Wait for timeout message in log
TIMEOUT_DETECTED=false
while true; do
    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))

    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "✗ Test timeout waiting for reaper"
        echo ""
        echo "=== Reaper Log ==="
        cat "$REAPER_LOG" 2>/dev/null || echo "(log file not found)"
        exit 1
    fi

    # Check for timeout fallback message
    if [ -f "$REAPER_LOG" ] && grep -q "Prompt detection failed" "$REAPER_LOG"; then
        if [ "$TIMEOUT_DETECTED" = "false" ]; then
            echo "✓ Prompt detection timeout detected (${ELAPSED}s)"
            TIMEOUT_DETECTED=true
        fi
    fi

    # Check for fallback message
    if [ -f "$REAPER_LOG" ] && grep -q "Falling back" "$REAPER_LOG"; then
        echo "✓ Fallback to fixed wait activated (${ELAPSED}s)"
    fi

    # Check for successful completion (reaper should complete even without prompt)
    if [ -f "$REAPER_LOG" ] && grep -q "Session archived successfully" "$REAPER_LOG"; then
        echo "✓ Reaper completed despite timeout (${ELAPSED}s)"
        break
    fi

    sleep 2
done

# Verify timing expectations
ELAPSED_FINAL=$(($(date +%s) - START_TIME))
echo ""
echo "Step 6: Verify timing expectations..."
echo "Total elapsed time: ${ELAPSED_FINAL}s"

# Should take at least 90s (prompt timeout) + 60s (fallback) = 150s
if [ $ELAPSED_FINAL -lt 140 ]; then
    echo "✗ Completed too quickly (expected ~150s, got ${ELAPSED_FINAL}s)"
    echo "This suggests fallback didn't actually wait"
    exit 1
fi

if [ $ELAPSED_FINAL -gt 180 ]; then
    echo "⚠️  Took longer than expected (${ELAPSED_FINAL}s > 180s)"
    echo "Still acceptable, just slower than ideal"
fi

echo "✓ Timing is reasonable (${ELAPSED_FINAL}s, expected ~150s)"

echo ""
echo "Step 7: Verify session archived despite timeout..."
ARCHIVE_DIR="$SESSIONS_DIR/.archive-old-format/$TEST_SESSION"

# Find the archived directory (may have timestamp suffix)
ACTUAL_ARCHIVE=$(find "$SESSIONS_DIR/.archive-old-format" -name "${TEST_SESSION}*" -type d 2>/dev/null | head -1)

if [ -z "$ACTUAL_ARCHIVE" ]; then
    echo "✗ Archived session not found"
    ls -la "$SESSIONS_DIR/.archive-old-format" 2>/dev/null || echo "(archive dir not found)"
    exit 1
fi

MANIFEST_PATH="$ACTUAL_ARCHIVE/manifest.yaml"

if grep -q "lifecycle: archived" "$MANIFEST_PATH"; then
    echo "✓ Session manifest shows lifecycle: archived"
else
    echo "✗ Session not archived in manifest"
    cat "$MANIFEST_PATH"
    exit 1
fi

echo ""
echo "Step 8: Verify fallback messages in log..."
if grep -q "Prompt detection failed" "$REAPER_LOG" && \
   grep -q "Falling back" "$REAPER_LOG"; then
    echo "✓ Reaper log shows timeout and fallback"
else
    echo "✗ Expected timeout/fallback messages not found"
    cat "$REAPER_LOG"
    exit 1
fi

echo ""
echo "=== Test Results ==="
echo "✓ All checks passed"
echo ""
echo "Reaper log excerpt:"
tail -15 "$REAPER_LOG"

echo ""
echo "🎉 Reaper Prompt Timeout E2E Test: PASSED"
echo ""
echo "Key findings:"
echo "- Prompt detection timed out as expected (~90s)"
echo "- Fallback to fixed wait activated (60s)"
echo "- Session archived successfully despite no prompt"
echo "- Total time: ${ELAPSED_FINAL}s (expected ~150s)"
exit 0
