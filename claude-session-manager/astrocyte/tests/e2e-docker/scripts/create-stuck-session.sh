#!/bin/bash
# Create a tmux session with simulated stuck state
# Usage: create-stuck-session.sh <session-name> <fixture-file>

set -euo pipefail

SESSION_NAME="$1"
FIXTURE_FILE="$2"

SESSIONS_DIR="/home/testuser/src/sessions"
TMUX_SOCKET="/tmp/csm.sock"
FIXTURES_DIR="/tests/fixtures"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Validate inputs
if [ -z "$SESSION_NAME" ]; then
    log_error "Session name is required"
    exit 1
fi

if [ -z "$FIXTURE_FILE" ]; then
    log_error "Fixture file is required"
    exit 1
fi

if [ ! -f "$FIXTURES_DIR/$FIXTURE_FILE" ]; then
    log_error "Fixture file not found: $FIXTURES_DIR/$FIXTURE_FILE"
    exit 1
fi

# Create session directory
SESSION_DIR="$SESSIONS_DIR/$SESSION_NAME"
mkdir -p "$SESSION_DIR"

# Create manifest.yaml
cat "$FIXTURES_DIR/manifest-template.yaml" | sed "s/{SESSION_NAME}/$SESSION_NAME/g" > "$SESSION_DIR/manifest.yaml"

log_info "Created manifest for session: $SESSION_NAME"

# Create tmux session
tmux -S "$TMUX_SOCKET" new-session -d -s "$SESSION_NAME" -c "$SESSION_DIR"

log_info "Created tmux session: $SESSION_NAME"

# Wait for session to initialize
sleep 0.5

# Load fixture content into tmux buffer and paste it
FIXTURE_CONTENT=$(cat "$FIXTURES_DIR/$FIXTURE_FILE")

# Use tmux load-buffer and paste-buffer to inject content
echo "$FIXTURE_CONTENT" | tmux -S "$TMUX_SOCKET" load-buffer -
tmux -S "$TMUX_SOCKET" paste-buffer -t "$SESSION_NAME"

log_info "Loaded fixture content from: $FIXTURE_FILE"

# Verify session exists
if tmux -S "$TMUX_SOCKET" has-session -t "$SESSION_NAME" 2>/dev/null; then
    log_info "Session created successfully: $SESSION_NAME"

    # Capture and display pane content for verification
    PANE_CONTENT=$(tmux -S "$TMUX_SOCKET" capture-pane -t "$SESSION_NAME" -p | tail -5)
    echo "Pane content (last 5 lines):"
    echo "$PANE_CONTENT"
else
    log_error "Failed to create session: $SESSION_NAME"
    exit 1
fi
