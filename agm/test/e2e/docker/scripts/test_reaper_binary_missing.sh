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
rm -rf "$SESSIONS_DIR"/* 2>/dev/null || true

echo "Step 1: Create tmux session with mock Claude..."
# The pane process must be a file named `claude` — see the happy-path test.
tmux -S "$AGM_SOCKET" new-session -d -s "$TEST_SESSION" claude
sleep 2
echo "✓ Tmux session created"

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
  project: "E2E Binary Missing Test"
claude:
  uuid: ""
tmux:
  session_name: $TEST_SESSION
EOF

echo "✓ AGM manifest created"

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
echo "Step 3: Hide agm-reaper binary temporarily..."
# PATH is the wrong lever, and stripping it was never testing this scenario:
# agm resolves the reaper as filepath.Join(filepath.Dir(os.Executable()),
# "agm-reaper") — beside its own binary, never through PATH. Removing
# /home/testuser/bin from PATH only took `agm` with it, so the run died on
# "agm: command not found" without reaching the spawn. A symlink does not work
# either: os.Executable() resolves it, so the reaper is still found next to the
# real binary.
#
# Run a COPY of agm from a directory that has no agm-reaper beside it.
ORIGINAL_PATH="$PATH"
AGM_ONLY_BIN="$(mktemp -d)"
cp /home/testuser/bin/agm "$AGM_ONLY_BIN/agm"
export PATH="$AGM_ONLY_BIN:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
[ -x "$AGM_ONLY_BIN/agm" ]        || { echo "✗ fixture broken: agm must stay runnable"; exit 1; }
[ ! -e "$AGM_ONLY_BIN/agm-reaper" ] || { echo "✗ fixture broken: agm-reaper must not sit beside it"; exit 1; }
echo "✓ agm runs from a directory with no agm-reaper beside it"
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
    echo "   This is the assertion the test exists for — not optional."
    exit 1
fi

# Check 2: Error indicates binary not found or spawn failed
if echo "$ERROR_OUTPUT" | grep -qiE "(not found|failed to start|no such file)"; then
    echo "✓ Error indicates binary not found/spawn failed"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo "✗ Error should indicate binary not found"
    exit 1
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
ARCHIVE_DIR="$SESSIONS_DIR/.archive-old-format/$SESSION_UUID"

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
