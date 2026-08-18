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
rm -rf "$SESSIONS_DIR"/* 2>/dev/null || true
rm -f "$REAPER_LOG" 2>/dev/null || true

echo "Step 1: Create tmux session with stuck Claude (never shows prompt)..."
# `claude --stuck` never reaches the prompt, exercising the reaper's
# prompt-detection timeout and its timer fallback. Same binary as the other
# tests: the pane process must be named `claude` for AGM to see a live
# harness rather than a zombie.
tmux -S "$AGM_SOCKET" new-session -d -s "$TEST_SESSION" claude --stuck
sleep 2  # Wait for mock Claude to start
echo "✓ Tmux session created with stuck Claude (socket: $AGM_SOCKET)"

echo ""
echo "Step 2: Create AGM session manifest..."
SESSION_UUID=$(uuidgen 2>/dev/null || echo "test-uuid-$(date +%s)")
# Keyed by session ID, not session name: ops.ArchiveSession looks for the
# legacy directory at <sessions-dir>/<session-id>/manifest.yaml and moves it to
# <sessions-dir>/.archive-old-format/<session-id>/. A name-keyed directory is
# invisible to that migration, so the archive assertions below never fired.
SESSION_DIR="$SESSIONS_DIR/$SESSION_UUID"
mkdir -p "$SESSION_DIR"
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

# The reaper resolves its target through the AGM lifecycle store, not through
# this manifest: ops.ArchiveSession looks the session up by identifier, and the
# reaper's archive preflight runs before it touches the pane. A fixture with
# only a manifest.yaml dies at preflight with AGM-001 and never reaps anything.
/home/testuser/bin/seed-session \
    --session-id "$SESSION_UUID" \
    --name "$TEST_SESSION" \
    --harness claude-code \
    --project "$SESSION_DIR"
echo "✓ AGM lifecycle record seeded: $SESSION_UUID"

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
# 90s prompt detection + 60s fallback gets only as far as sending /exit. A
# stuck harness does not act on it, so the reaper then runs the full pane-close
# escalation — wait for close, SIGTERM, SIGKILL, kill-session — before it can
# archive. Budgeting the old ~150s cut the run off mid-escalation and reported
# a timeout for a reaper that was working exactly as designed.
echo "Total expected time: ~230s (90s prompt + 60s fallback + pane-close escalation)"

/home/testuser/bin/agm-reaper \
    --session "$TEST_SESSION" \
    --session-id "$SESSION_UUID" \
    --log-file "$REAPER_LOG" \
    --sessions-dir "$SESSIONS_DIR" &

REAPER_PID=$!
echo "✓ Reaper spawned with PID: $REAPER_PID"

echo ""
echo "Step 5: Monitor reaper log for timeout fallback (max 300s)..."
START_TIME=$(date +%s)
TIMEOUT=300

# Wait for timeout message in log
TIMEOUT_DETECTED=false
FALLBACK_DETECTED=false
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

    # Check for fallback message. Latched like the one above: unlatched, it
    # re-printed every 2s for the rest of the run.
    if [ -f "$REAPER_LOG" ] && grep -q "Falling back" "$REAPER_LOG"; then
        if [ "$FALLBACK_DETECTED" = "false" ]; then
            echo "✓ Fallback to fixed wait activated (${ELAPSED}s)"
            FALLBACK_DETECTED=true
        fi
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

if [ $ELAPSED_FINAL -gt 280 ]; then
    echo "⚠️  Took longer than expected (${ELAPSED_FINAL}s > 280s)"
    echo "Still acceptable, just slower than ideal"
fi

echo "✓ Timing is reasonable (${ELAPSED_FINAL}s, expected ~230s)"

echo ""
echo "Step 7: Verify session archived despite timeout..."
# The legacy directory is keyed by session ID, and may carry a timestamp
# suffix when a previous archive of the same id already exists.
ACTUAL_ARCHIVE=$(find "$SESSIONS_DIR/.archive-old-format" -name "${SESSION_UUID}*" -type d 2>/dev/null | head -1)

if [ -z "$ACTUAL_ARCHIVE" ]; then
    echo "✗ Archived session directory not found"
    ls -la "$SESSIONS_DIR/.archive-old-format" 2>/dev/null || echo "(archive dir not found)"
    exit 1
fi
echo "✓ Legacy session directory moved to $ACTUAL_ARCHIVE"

# Assert the archive in the lifecycle store — the only thing ArchiveSession
# updates. The legacy directory is MOVED verbatim, so its manifest.yaml still
# carries whatever lifecycle it had on disk.
SESSION_STATUS=$(agm session get "$SESSION_UUID" -o json 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["status"])' 2>/dev/null || echo "")
if [ "$SESSION_STATUS" = "archived" ]; then
    echo "✓ Lifecycle store reports the session archived"
else
    echo "✗ Lifecycle store reports status '${SESSION_STATUS:-<unavailable>}', expected 'archived'"
    agm session get "$SESSION_UUID" -o json 2>&1 | head -5
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
