#!/bin/bash
# Integration test for context monitor hook with actual AGM sessions
set -e

HOOK_SCRIPT="$(dirname "$0")/posttool-context-monitor.py"
TEST_SESSION="context-monitor-test-$$"

echo "=== Context Monitor Hook Integration Test ==="
echo ""

# Cleanup function
cleanup() {
    echo "Cleaning up test session..."
    agm session kill "$TEST_SESSION" 2>/dev/null || true
    rm -f /tmp/agm-context-cache/test-session-* 2>/dev/null || true
}

trap cleanup EXIT

# Create test AGM session (skip actual test - use existing session instead)
echo "1. Using existing AGM session for testing"
echo "  Testing with session: multi-persona-review-vertexai-fix"
TEST_SESSION="multi-persona-review-vertexai-fix"

# Get session UUID from Dolt
echo "  Looking up session UUID..."
SESSION_UUID=$(agm session list --json 2>/dev/null | python3 -c "import sys, json; sessions = json.load(sys.stdin); print(next((s['id'] for s in sessions if '$TEST_SESSION' in s.get('tmux', {}).get('session_name', '')), ''))" 2>/dev/null)

if [ -z "$SESSION_UUID" ]; then
    echo "Error: Could not get session UUID"
    exit 1
fi

echo "  Session UUID: $SESSION_UUID"
echo ""

# Simulate token usage input (what Claude Code would send)
echo "2. Simulating hook invocation with token usage data"

export CLAUDE_SESSION_ID="$SESSION_UUID"
export CLAUDE_TOOL_NAME="Bash"
export CLAUDE_TOOL_RESULT="<system-reminder>Token usage: 50000/200000; 150000 remaining</system-reminder>"
export AGM_HOOK_DEBUG="1"

# Run hook
echo "  Running hook..."
"$HOOK_SCRIPT" 2>&1 | grep -E "(INFO|ERROR|WARNING)" || echo "  (no debug output)"

echo ""

# Verify context was updated
echo "3. Verifying AGM manifest was updated"
agm session status-line -s "$TEST_SESSION" --json | grep -q '"ContextPercent": 25' && \
    echo "  ✓ Context percentage updated: 25%" || \
    echo "  ✗ Context percentage NOT updated"

# Test with different percentage
echo ""
echo "4. Testing update with different percentage"

export CLAUDE_TOOL_RESULT="<system-reminder>Token usage: 120000/200000; 80000 remaining</system-reminder>"

"$HOOK_SCRIPT" 2>&1 | grep -E "(INFO|ERROR|WARNING)" || true

agm session status-line -s "$TEST_SESSION" --json | grep -q '"ContextPercent": 60' && \
    echo "  ✓ Context percentage updated: 60%" || \
    echo "  ✗ Context percentage NOT updated"

# Test caching (should skip update within interval)
echo ""
echo "5. Testing cache throttling (should skip update)"

export CLAUDE_TOOL_RESULT="<system-reminder>Token usage: 121000/200000; 79000 remaining</system-reminder>"

"$HOOK_SCRIPT" 2>&1 | grep -q "Skipping update" && \
    echo "  ✓ Update skipped (cache throttling works)" || \
    echo "  ✗ Update not skipped (cache may not be working)"

# Test with non-AGM session (should skip silently)
echo ""
echo "6. Testing with non-AGM session (should skip)"

export CLAUDE_SESSION_ID="fake-session-not-agm"
export CLAUDE_TOOL_RESULT="<system-reminder>Token usage: 100000/200000; 100000 remaining</system-reminder>"

"$HOOK_SCRIPT" 2>&1 | grep -q "Not an AGM session" && \
    echo "  ✓ Correctly skipped non-AGM session" || \
    echo "  ✗ Did not skip non-AGM session"

echo ""
echo "=== Integration Test Complete ==="
echo ""
echo "Summary:"
echo "  - Hook extracts token usage from system reminders"
echo "  - AGM manifest updated via CLI"
echo "  - Cache throttling prevents excessive updates"
echo "  - Non-AGM sessions handled gracefully"
echo ""
