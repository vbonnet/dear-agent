#!/bin/bash

# claude-auto-detect.sh - Auto-detect Claude UUID from tmux session
# Experimental: Tries to find Claude session UUID for a running tmux session

# detect_claude_uuid_from_tmux(tmux_session_name)
# Returns: Claude UUID if found, empty if not detected
detect_claude_uuid_from_tmux() {
    local tmux_name="$1"

    # Method 1: Check if claude process is running in this tmux session
    local pids
    pids=$(tmux list-panes -t "$tmux_name" -F '#{pane_pid}' 2>/dev/null || true)

    if [[ -z "$pids" ]]; then
        return 1
    fi

    for pid in $pids; do
        # Check if this is a claude process
        if ps -p "$pid" -o comm= 2>/dev/null | grep -q "claude"; then
            # Try to find session UUID in process environment or working directory
            # This is experimental - Claude might store UUID in env vars or cwd

            # Method 1a: Check /proc/$pid/environ for SESSION_ID or similar
            if [[ -f "/proc/$pid/environ" ]]; then
                local env_uuid
                env_uuid=$(tr '\0' '\n' < "/proc/$pid/environ" 2>/dev/null | grep -E "CLAUDE.*SESSION|SESSION.*ID" | grep -oE '[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}' | head -1 || true)

                if [[ -n "$env_uuid" ]]; then
                    echo "$env_uuid"
                    return 0
                fi
            fi

            # Method 1b: Check working directory (might contain session UUID)
            if [[ -L "/proc/$pid/cwd" ]]; then
                local cwd
                cwd=$(readlink "/proc/$pid/cwd" 2>/dev/null || true)

                local cwd_uuid
                cwd_uuid=$(echo "$cwd" | grep -oE '[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}' | head -1 || true)

                if [[ -n "$cwd_uuid" ]]; then
                    echo "$cwd_uuid"
                    return 0
                fi
            fi
        fi
    done

    # Method 2: Match by recent activity in history.jsonl
    # Find sessions that match this tmux session's working directory
    local tmux_cwd
    tmux_cwd=$(tmux display-message -t "$tmux_name" -p '#{pane_current_path}' 2>/dev/null || true)

    if [[ -n "$tmux_cwd" ]] && [[ -f "$HOME/.claude/history.jsonl" ]]; then
        # Find most recent Claude session with this project path
        local uuid
        uuid=$(cat "$HOME/.claude/history.jsonl" | python3 -c "
import json, sys
from datetime import datetime

target_path = '$tmux_cwd'
cutoff = datetime.now().timestamp() - 3600  # Last hour
sessions = {}

for line in sys.stdin:
    try:
        obj = json.loads(line.strip())
        sid = obj.get('sessionId')
        project = obj.get('project', '')
        ts = obj.get('timestamp', 0) / 1000

        if sid and ts > cutoff and project == target_path:
            if sid not in sessions or ts > sessions[sid]:
                sessions[sid] = ts
    except: pass

if sessions:
    # Return most recent
    most_recent = max(sessions.items(), key=lambda x: x[1])
    print(most_recent[0])
" 2>/dev/null || true)

        if [[ -n "$uuid" ]]; then
            echo "$uuid"
            return 0
        fi
    fi

    return 1
}

# Export function
export -f detect_claude_uuid_from_tmux
