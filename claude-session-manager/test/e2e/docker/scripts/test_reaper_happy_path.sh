#!/bin/bash
set -e

echo "=== Reaper Happy Path E2E Test ==="
echo ""

# Setup paths
export PATH="/home/testuser/bin:$PATH"
SESSIONS_DIR="/home/testuser/sessions"
TEST_SESSION="test-reaper-session"
REAPER_LOG="/tmp/agm-reaper-${TEST_SESSION}.log"

# Cleanup from previous runs
tmux kill-session -t "$TEST_SESSION" 2>/dev/null || true
rm -rf "$SESSIONS_DIR/$TEST_SESSION" 2>/dev/null || true
rm -f "$REAPER_LOG" 2>/dev/null || true

echo "Step 1: Create tmux session..."
tmux new-session -d -s "$TEST_SESSION"
echo "✓ Tmux session created: $TEST_SESSION"

echo ""
echo "Step 2: Launch mock Claude in tmux pane..."
tmux send-keys -t "$TEST_SESSION" "python3 /home/testuser/tests/mock_claude.py" C-m
sleep 2  # Wait for mock Claude to start
echo "✓ Mock Claude launched"

echo ""
echo "Step 3: Create AGM session manifest..."
# Create sessions directory
mkdir -p "$SESSIONS_DIR"

# Create manifest manually (simpler than calling agm for test)
SESSION_UUID=$(uuidgen 2>/dev/null || echo "test-uuid-$(date +%s)")
SESSION_DIR="$SESSIONS_DIR/$SESSION_UUID"
mkdir -p "$SESSION_DIR"

cat > "$SESSION_DIR/manifest.yaml" <<EOF
id: $SESSION_UUID
name: $TEST_SESSION
tmux_session: $TEST_SESSION
tmux_pane: $TEST_SESSION:0.0
lifecycle: active
created_at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF

echo "✓ AGM manifest created: $SESSION_DIR/manifest.yaml"

echo ""
echo "Step 4: Verify mock Claude is ready (check for prompt)..."
sleep 1
CAPTURE=$(tmux capture-pane -t "$TEST_SESSION" -p)
if echo "$CAPTURE" | grep -q ">"; then
    echo "✓ Mock Claude showing prompt"
else
    echo "✗ Mock Claude prompt not detected"
    echo "Captured output:"
    echo "$CAPTURE"
    exit 1
fi

echo ""
echo "Step 5: Spawn async archive with agm-reaper..."
# Note: agm session archive doesn't exist in test environment
# Call agm-reaper directly instead
/home/testuser/bin/agm-reaper \
    --session "$TEST_SESSION" \
    --log-file "$REAPER_LOG" \
    --sessions-dir "$SESSIONS_DIR" &

REAPER_PID=$!
echo "✓ Reaper spawned with PID: $REAPER_PID"

echo ""
echo "Step 6: Monitor reaper log for completion (timeout: 120s)..."
START_TIME=$(date +%s)
TIMEOUT=120

while true; do
    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))

    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "✗ Timeout waiting for reaper to complete"
        echo ""
        echo "=== Reaper Log ==="
        cat "$REAPER_LOG" 2>/dev/null || echo "(log file not found)"
        echo ""
        echo "=== Tmux Session ==="
        tmux capture-pane -t "$TEST_SESSION" -p 2>/dev/null || echo "(session not found)"
        exit 1
    fi

    if [ -f "$REAPER_LOG" ] && grep -q "Session archived successfully" "$REAPER_LOG"; then
        echo "✓ Reaper completed successfully (${ELAPSED}s)"
        break
    fi

    sleep 1
done

echo ""
echo "Step 7: Verify session archived..."
MANIFEST_PATH="$SESSION_DIR/manifest.yaml"
if [ ! -f "$MANIFEST_PATH" ]; then
    echo "✗ Manifest not found at $MANIFEST_PATH"
    exit 1
fi

if grep -q "lifecycle: archived" "$MANIFEST_PATH"; then
    echo "✓ Session manifest shows lifecycle: archived"
else
    echo "✗ Session not archived in manifest"
    cat "$MANIFEST_PATH"
    exit 1
fi

echo ""
echo "Step 8: Verify tmux session no longer exists..."
if tmux has-session -t "$TEST_SESSION" 2>/dev/null; then
    echo "✗ Tmux session still exists (should be closed)"
    exit 1
else
    echo "✓ Tmux session closed"
fi

echo ""
echo "=== Test Results ==="
echo "✓ All checks passed"
echo ""
echo "Reaper log excerpt:"
tail -10 "$REAPER_LOG"

echo ""
echo "🎉 Reaper Happy Path E2E Test: PASSED"
exit 0
