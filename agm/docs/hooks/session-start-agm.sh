#!/bin/bash
# SessionStart hook for AGM (AI Agent Manager)
#
# This hook creates a ready-file signal when Claude CLI is fully initialized
# and ready to accept slash commands. This replaces fragile text-parsing-based
# prompt detection with a deterministic file-based signal.
#
# Installation:
#   1. Copy this file to: ~/.config/claude/hooks/session-start-agm.sh
#   2. Make executable: chmod +x ~/.config/claude/hooks/session-start-agm.sh
#   3. Add to ~/.config/claude/config.yaml:
#
#      hooks:
#        SessionStart:
#          - name: agm-ready-signal
#            command: ~/.config/claude/hooks/session-start-agm.sh
#
# How it works:
#   - agm sets AGM_SESSION_NAME=<session-name> when starting Claude
#   - This hook detects that variable and creates ~/.agm/claude-ready-<session-name>
#   - agm waits for this file before sending initialization commands
#   - This ensures commands are sent only when Claude is truly ready

# Only run for AGM-managed sessions
if [ -z "$AGM_SESSION_NAME" ]; then
    # Not an AGM session, skip
    exit 0
fi

# Ensure .agm directory exists
mkdir -p ~/.agm

# Remove pending marker (if exists)
rm -f ~/.agm/pending-${AGM_SESSION_NAME}

# Create ready signal
touch ~/.agm/claude-ready-${AGM_SESSION_NAME}

# Debug logging to stderr (visible in agm debug logs)
echo "[AGM Hook] Claude ready signal created for session: ${AGM_SESSION_NAME}" >&2

# Success
exit 0
