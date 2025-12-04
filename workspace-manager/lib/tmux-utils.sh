#!/bin/bash

# tmux-utils.sh - Tmux session control and logging utilities
# Part of unified session management CLI

# ensure_tmux_and_resume(session_name, worktree_path, claude_uuid)
# Create or attach to tmux session with Claude auto-resumed
# This function does NOT return - it execs tmux attach
ensure_tmux_and_resume() {
    local session_name="$1"
    local worktree_path="$2"
    local claude_uuid="$3"

    # Validate parameters
    if [[ -z "$session_name" ]] || [[ -z "$worktree_path" ]] || [[ -z "$claude_uuid" ]]; then
        log_error "ensure_tmux_and_resume: All parameters required"
        log_error "Usage: ensure_tmux_and_resume SESSION_NAME WORKTREE_PATH CLAUDE_UUID"
        return 1
    fi

    # Check tmux is available
    if ! command -v tmux &>/dev/null; then
        log_error "tmux is not installed"
        log_error "Install with: sudo apt-get install tmux"
        return 5  # Dependency missing
    fi

    # Check if session already exists
    if ! tmux has-session -t "$session_name" 2>/dev/null; then
        log_info "Creating tmux session '$session_name' with Claude"

        # Create new session with Claude command (user-suggested approach)
        tmux new-session -d -s "$session_name" \
            -c "$worktree_path" \
            "claude --resume $claude_uuid"

        if [[ $? -ne 0 ]]; then
            log_error "Failed to create tmux session"
            return 1
        fi
    else
        log_info "Attaching to existing tmux session: $session_name"
    fi

    # Log resume action (Review Condition #6)
    log_resume_action "$session_name" "$claude_uuid" "attached"

    # Attach to session (automatic per user request)
    # Note: This replaces current process
    exec tmux attach -t "$session_name"
}

# log_resume_action(session_id, claude_uuid, action, [details])
# Log session resume operations for audit and debugging
# Review Condition #6: Resume action logging
log_resume_action() {
    local session_id="$1"
    local claude_uuid="$2"
    local action="$3"
    local details="${4:-}"

    local log_file="${RESUME_LOG:-$HOME/sessions/.resume-log}"
    local timestamp
    timestamp=$(date -Iseconds)

    # Create log file if doesn't exist
    if [[ ! -f "$log_file" ]]; then
        mkdir -p "$(dirname "$log_file")"
        cat > "$log_file" <<EOF
# Resume Action Log
# Format: timestamp | session_id | claude_uuid | action | details
#
# This log tracks all session resume operations for audit and debugging.
#
EOF
    fi

    # Append entry
    echo "$timestamp | $session_id | $claude_uuid | $action | $details" >> "$log_file"
}

# get_unique_tmux_name(desired_name)
# Handle tmux session name conflicts
# Returns: Available name (may be modified)
get_unique_tmux_name() {
    local desired="$1"

    # Check if name is available
    if ! tmux has-session -t "$desired" 2>/dev/null; then
        echo "$desired"
        return 0
    fi

    # Conflict: Offer alternatives
    echo "" >&2
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
    echo "Tmux Session Name Conflict" >&2
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
    echo "" >&2
    echo "Session name '$desired' already exists." >&2
    echo "" >&2
    echo "This might mean:" >&2
    echo "  - An existing Claude session is running in that tmux" >&2
    echo "  - A stale tmux session exists" >&2
    echo "" >&2
    echo "Options:" >&2
    echo "  1. Use alternate name: ${desired}-alt" >&2
    echo "  2. Attach to existing session (may have different Claude session)" >&2
    echo "  3. Cancel" >&2
    echo "" >&2
    read -p "Select option (1-3): " -n 1 -r choice >&2
    echo "" >&2
    echo "" >&2

    case "$choice" in
        1)
            echo "${desired}-alt"
            return 0
            ;;
        2)
            echo "$desired"
            return 0
            ;;
        3)
            log_info "Cancelled by user"
            return 1
            ;;
        *)
            log_error "Invalid choice: $choice"
            return 1
            ;;
    esac
}
