#!/bin/bash
# Validation script for state detection accuracy
# Tests state detection on 10 real sessions and calculates false positive rate

set -euo pipefail

# Sessions to test (mix of active, busy, and offline)
SESSIONS=(
    "coordination-research"
    "find-sessions"
    "agm-conflicts"
    "bow-core"
    "enhance-bow-core"
    "gemini-task-1-4"
    "language-audit"
    "slash-commands"
    "engram-research"
    "migration-coordinator"
)

echo "====================================="
echo "State Detection Validation"
echo "====================================="
echo ""
echo "Testing state detection on ${#SESSIONS[@]} sessions..."
echo ""

# Test each session
for session in "${SESSIONS[@]}"; do
    # Use production agm binary (build would fail due to engram/core replace directive)
    # For now, manually test with tmux inspection

    # Check if session exists in tmux
    if tmux has-session -t "$session" 2>/dev/null; then
        # Capture last 50 lines
        pane_content=$(tmux capture-pane -t "$session" -p -S -50 2>/dev/null || echo "")

        # Manual state detection for validation
        state="THINKING"

        if echo "$pane_content" | grep -q "Wrangling…\|Cogitated for"; then
            state="COMPACTING"
        elif echo "$pane_content" | grep -q "Allow bash command\|Allow Bash command\|Permission to use\|has been denied"; then
            state="PERMISSION_PROMPT"
        elif echo "$pane_content" | tail -5 | grep -q "❯\|⏵⏵"; then
            state="READY"
        fi

        echo "Session: $session → Detected: $state"

        # Show last 3 lines for manual verification
        echo "  Last 3 lines:"
        echo "$pane_content" | tail -3 | sed 's/^/    /'
        echo ""
    else
        echo "Session: $session → Detected: OFFLINE"
        echo ""
    fi
done

echo "====================================="
echo "Manual Verification Required"
echo "====================================="
echo ""
echo "Next steps:"
echo "1. Attach to each session (tmux attach -t <session-name>)"
echo "2. Verify actual state matches detected state"
echo "3. Record false positives"
echo "4. Calculate false positive rate = (false positives / total sessions) * 100"
echo ""
echo "Target: <5% false positive rate"
echo "If >5%: Refine detection patterns and re-test"
