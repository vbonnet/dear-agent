#!/bin/bash
set -e

echo "=== Reaper Happy Path E2E Test ==="
echo ""

# Setup paths
export PATH="/home/testuser/bin:$PATH"
SESSIONS_DIR="/home/testuser/sessions"
TEST_SESSION="test-reaper-session"
REAPER_LOG="/tmp/agm-reaper-${TEST_SESSION}.log"
AGM_SOCKET="/tmp/agm.sock"

# Cleanup from previous runs
tmux -S "$AGM_SOCKET" kill-session -t "$TEST_SESSION" 2>/dev/null || true
rm -rf "$SESSIONS_DIR"/* 2>/dev/null || true
rm -f "$REAPER_LOG" 2>/dev/null || true

echo "Step 1: Create tmux session with mock Claude..."
# The pane process must be a file named `claude`: AGM matches the pane
# process COMM against the harness to decide liveness, and Linux takes COMM
# from the executable file name — an interpreted mock reports `python3` and
# the session computes as "zombie" rather than "active".
tmux -S "$AGM_SOCKET" new-session -d -s "$TEST_SESSION" claude
sleep 2  # Wait for mock Claude to start
echo "✓ Tmux session created with mock Claude (socket: $AGM_SOCKET)"

echo ""
echo "Step 2: Create AGM session manifest..."
# Create sessions directory
mkdir -p "$SESSIONS_DIR"

# Generate the session ID first: the session directory is keyed by it.
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
  project: "E2E Test"
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
echo "Step 3: Verify mock Claude is ready (check for prompt)..."
sleep 1
CAPTURE=$(tmux -S "$AGM_SOCKET" capture-pane -t "$TEST_SESSION" -p)
if echo "$CAPTURE" | grep -q "❯"; then
    echo "✓ Mock Claude showing prompt"
else
    echo "✗ Mock Claude prompt not detected"
    echo "Captured output:"
    echo "$CAPTURE"
    exit 1
fi

echo ""
echo "Step 4: Spawn async archive with agm-reaper..."
# Note: agm session archive doesn't exist in test environment
# Call agm-reaper directly instead
/home/testuser/bin/agm-reaper \
    --session "$TEST_SESSION" \
    --session-id "$SESSION_UUID" \
    --log-file "$REAPER_LOG" \
    --sessions-dir "$SESSIONS_DIR" &

REAPER_PID=$!
echo "✓ Reaper spawned with PID: $REAPER_PID"

echo ""
echo "Step 5: Monitor reaper log for completion (timeout: 120s)..."
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
        tmux -S "$AGM_SOCKET" capture-pane -t "$TEST_SESSION" -p 2>/dev/null || echo "(session not found)"
        exit 1
    fi

    if [ -f "$REAPER_LOG" ] && grep -q "Session archived successfully" "$REAPER_LOG"; then
        echo "✓ Reaper completed successfully (${ELAPSED}s)"
        break
    fi

    sleep 1
done

echo ""
echo "Step 6: Verify session archived..."
# Session should be moved to .archive-old-format/ subdirectory
ARCHIVE_DIR="$SESSIONS_DIR/.archive-old-format/$SESSION_UUID"
MANIFEST_PATH="$ARCHIVE_DIR/manifest.yaml"

if [ ! -f "$MANIFEST_PATH" ]; then
    echo "✗ Manifest not found at $MANIFEST_PATH"
    echo "Expected archive directory: $ARCHIVE_DIR"
    ls -la "$SESSIONS_DIR" 2>/dev/null || echo "(sessions dir not found)"
    ls -la "$SESSIONS_DIR/.archive-old-format" 2>/dev/null || echo "(archive dir not found)"
    exit 1
fi

# The archived state lives in the lifecycle store, which is the only thing
# ops.ArchiveSession updates. The legacy directory is MOVED verbatim, so its
# manifest.yaml still carries whatever lifecycle it had on disk — asserting
# `lifecycle: archived` in the moved file tested nothing the product does.
SESSION_STATUS=$(agm session get "$SESSION_UUID" -o json 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["status"])' 2>/dev/null || echo "")
if [ "$SESSION_STATUS" = "archived" ]; then
    echo "✓ Lifecycle store reports the session archived"
else
    echo "✗ Lifecycle store reports status '${SESSION_STATUS:-<unavailable>}', expected 'archived'"
    agm session get "$SESSION_UUID" -o json 2>&1 | head -5
    exit 1
fi

# Verify original session directory no longer exists (was moved)
if [ -d "$SESSION_DIR" ]; then
    echo "✗ Original session directory still exists (should be moved)"
    exit 1
fi
echo "✓ Original session directory moved to archive"

echo ""
echo "Step 7: Verify tmux session no longer exists..."
if tmux -S "$AGM_SOCKET" has-session -t "$TEST_SESSION" 2>/dev/null; then
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
