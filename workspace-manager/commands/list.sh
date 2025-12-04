#!/bin/bash

set -euo pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARENT_DIR="$(dirname "$SCRIPT_DIR")"
LIB_DIR="$PARENT_DIR/lib"

# Source required libraries
source "$LIB_DIR/common-utils.sh"
source "$LIB_DIR/path-utils.sh"
source "$LIB_DIR/manifest-utils.sh"
source "$LIB_DIR/claude-discovery.sh"

# Show help
show_help() {
    cat <<EOF
Usage: session list [options]

List all Claude and workspace sessions.

Options:
  --claude       Show only Claude sessions
  --workspace    Show only workspace sessions
  --active       Show only active sessions
  --stale        Show only stale sessions
  -h, --help     Show this help message
  --verbose      Enable verbose output

Examples:
  session list
  session list --claude
  session list --active

Output Format:
  UUID (truncated) | Workspace ID              | Tmux    | Last Activity
  c86ffd41-cbcc    | github.com-user-repo-main | claude-1| 2025-12-03 17:30

See also:
  session resume   Resume a session
  session sync     Discover and sync Claude sessions
EOF
}

# truncate_uuid(uuid)
# Truncate UUID to first 18 characters for display
truncate_uuid() {
    local uuid="$1"
    echo "${uuid:0:18}"
}

# format_timestamp_from_iso(iso_timestamp)
# Format ISO timestamp to human-readable format
format_timestamp_from_iso() {
    local iso="$1"

    if [[ -z "$iso" ]]; then
        echo "Never"
        return
    fi

    # Convert ISO to readable format
    date -d "$iso" '+%Y-%m-%d %H:%M' 2>/dev/null || echo "Unknown"
}

# list_claude_sessions(show_active, show_stale)
# List all sessions with Claude integration
list_claude_sessions() {
    local show_active="${1:-false}"
    local show_stale="${2:-false}"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    if [[ ! -d "$sessions_dir" ]]; then
        log_info "No sessions directory found"
        return 0
    fi

    echo ""
    echo "Claude Sessions"
    echo ""
    printf "%-18s | %-27s | %-8s | %s\n" "UUID (truncated)" "Workspace ID" "Tmux" "Last Activity"
    echo "─────────────────────────────────────────────────────────────────────────────"

    local count=0

    for manifest in "$sessions_dir"/*/manifest.yaml; do
        if [[ ! -f "$manifest" ]]; then
            continue
        fi

        # Check if has Claude section
        local claude_uuid
        claude_uuid=$(read_claude_session_id "$manifest")

        if [[ -z "$claude_uuid" ]]; then
            continue  # Skip workspace-only sessions
        fi

        # Read session info
        local session_id
        local tmux_name
        local last_activity
        local status

        session_id=$(read_manifest_field "$manifest" "session_id")
        tmux_name=$(read_tmux_session_name "$manifest")
        last_activity=$(read_manifest_nested "$manifest" "claude" "last_activity")
        status=$(read_manifest_field "$manifest" "status")

        # Filter by active/stale if requested
        if [[ "$show_active" == true ]] && [[ "$status" != "active" ]]; then
            continue
        fi

        if [[ "$show_stale" == true ]] && [[ "$status" == "active" ]]; then
            continue
        fi

        # Display row
        printf "%-18s | %-27s | %-8s | %s\n" \
            "$(truncate_uuid "$claude_uuid")" \
            "$session_id" \
            "${tmux_name:-none}" \
            "$(format_timestamp_from_iso "$last_activity")"

        ((count++)) || true  # Prevent exit on count=0 with set -e
    done

    echo ""
    echo "Total: $count Claude sessions"
    echo ""
}

# list_workspace_sessions(show_active, show_stale)
# List all workspace-only sessions (no Claude)
list_workspace_sessions() {
    local show_active="${1:-false}"
    local show_stale="${2:-false}"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    if [[ ! -d "$sessions_dir" ]]; then
        return 0
    fi

    echo ""
    echo "Workspace-Only Sessions"
    echo ""
    printf "%-30s | %-10s | %s\n" "Workspace ID" "Status" "Last Activity"
    echo "──────────────────────────────────────────────────────────────"

    local count=0

    for manifest in "$sessions_dir"/*/manifest.yaml; do
        if [[ ! -f "$manifest" ]]; then
            continue
        fi

        # Check if has NO Claude section
        local claude_uuid
        claude_uuid=$(read_claude_session_id "$manifest")

        if [[ -n "$claude_uuid" ]]; then
            continue  # Skip Claude sessions
        fi

        # Read session info
        local session_id
        local status
        local last_activity

        session_id=$(read_manifest_field "$manifest" "session_id")
        status=$(read_manifest_field "$manifest" "status")
        last_activity=$(read_manifest_field "$manifest" "last_activity")

        # Filter by active/stale if requested
        if [[ "$show_active" == true ]] && [[ "$status" != "active" ]]; then
            continue
        fi

        if [[ "$show_stale" == true ]] && [[ "$status" == "active" ]]; then
            continue
        fi

        # Display row
        printf "%-30s | %-10s | %s\n" \
            "$session_id" \
            "$status" \
            "$(format_timestamp_from_iso "$last_activity")"

        ((count++)) || true  # Prevent exit on count=0 with set -e
    done

    echo ""
    echo "Total: $count workspace-only sessions"
    echo ""
}

# Main command logic
main() {
    local show_claude=false
    local show_workspace=false
    local show_active=false
    local show_stale=false

    # Parse options
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --claude)
                show_claude=true
                shift
                ;;
            --workspace)
                show_workspace=true
                shift
                ;;
            --active)
                show_active=true
                shift
                ;;
            --stale)
                show_stale=true
                shift
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                return 2
                ;;
        esac
    done

    # Default: show all
    if [[ "$show_claude" == false ]] && [[ "$show_workspace" == false ]]; then
        show_claude=true
        show_workspace=true
    fi

    # List Claude sessions
    if [[ "$show_claude" == true ]]; then
        list_claude_sessions "$show_active" "$show_stale"
    fi

    # List workspace sessions
    if [[ "$show_workspace" == true ]]; then
        list_workspace_sessions "$show_active" "$show_stale"
    fi
}

# Parse --help before main
if [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
    show_help
    exit 0
fi

main "$@"
