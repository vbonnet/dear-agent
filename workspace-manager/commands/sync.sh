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
Usage: session sync [options]

Discover Claude sessions from history.jsonl and sync with manifests.

By default, only shows active sessions (>20 messages, >24h duration).
This filters out test sessions and focuses on long-running work.

This command:
  1. Parses ~/.claude/history.jsonl for all Claude sessions
  2. Deduplicates sessions (keeps most recent activity per UUID)
  3. Matches sessions to existing workspace manifests
  4. Identifies orphaned sessions (no manifest)
  5. Offers to create manifests for orphaned sessions

Options:
  -h, --help       Show this help message
  --verbose        Enable verbose output
  --all            Show all sessions (including short/test sessions)
  --days <N>       Only show sessions active in last N days

Examples:
  session sync                    # Active sessions only (default)
  session sync --all              # All sessions (including 1-message tests)
  session sync --days 90          # Active sessions from last 90 days

See also:
  session list    List all sessions
  session resume  Resume a session
EOF
}

# format_timestamp(unix_timestamp_ms)
# Convert Unix timestamp (milliseconds) to human-readable format
format_timestamp() {
    local ts_ms="$1"

    # Convert milliseconds to seconds
    local ts_sec=$((ts_ms / 1000))

    # Format using date command
    date -d "@$ts_sec" '+%Y-%m-%d %H:%M' 2>/dev/null || echo "Unknown"
}

# map_session_to_workspace(uuid, project, timestamp)
# Create a new manifest for an orphaned Claude session
map_session_to_workspace() {
    local uuid="$1"
    local project="$2"
    local timestamp="$3"

    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    # Generate session ID from project path
    # e.g., /home/user/projects/repo → repo
    local session_id
    session_id=$(basename "$project")

    # Check if this session ID already exists
    local counter=1
    local final_session_id="$session_id"
    while [[ -d "$sessions_dir/$final_session_id" ]]; do
        final_session_id="${session_id}-${counter}"
        ((counter++)) || true
    done

    log_info "Creating manifest for session: $final_session_id"
    log_info "Project: $project"
    log_info "Claude UUID: $uuid"

    # Create session directory
    mkdir -p "$sessions_dir/$final_session_id"

    # Create basic manifest
    local manifest="$sessions_dir/$final_session_id/manifest.yaml"
    local started_at
    started_at=$(date -d "@$((timestamp / 1000))" -Iseconds 2>/dev/null || date -Iseconds)

    cat > "$manifest" <<EOF
session_id: $final_session_id
status: active
created_at: $(date -Iseconds)
last_activity: $started_at

repository:
  url: ""  # Unknown - update manually if needed

worktree:
  path: $project
  branch: ""  # Unknown

claude:
  session_id: $uuid
  session_env_path: $HOME/.claude/session-env/$uuid
  file_history_path: $HOME/.claude/file-history/$uuid
  started_at: $started_at
  last_activity: $started_at
EOF

    log_success "Created manifest: $manifest"
    echo ""
}

# migrate_orphaned_sessions(orphan_list)
# Interactively migrate orphaned sessions
# Review Condition #5: Progress tracking
migrate_orphaned_sessions() {
    local -a orphan_sessions=("$@")
    local total=${#orphan_sessions[@]}
    local current=0

    if [[ $total -eq 0 ]]; then
        log_info "No orphaned sessions to migrate"
        return 0
    fi

    echo "Found $total orphaned Claude sessions to map"
    echo ""

    for session_data in "${orphan_sessions[@]}"; do
        current=$((current + 1))

        # Parse session data
        IFS='|' read -r uuid project timestamp <<< "$session_data"

        # Progress indicator (Condition #5)
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Mapping session $current/$total"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "Claude UUID: $uuid"
        echo "Project: $project"
        echo "Last activity: $(format_timestamp "$timestamp")"
        echo ""

        # Validate session directories
        if ! validate_claude_session_dirs "$uuid"; then
            log_warn "Skipping invalid session (session-env missing or empty)"
            echo ""
            continue
        fi

        # Prompt for mapping
        read -p "Map to workspace session? (y/N/s=skip all): " -n 1 -r
        echo ""

        if [[ $REPLY =~ ^[Ss]$ ]]; then
            echo "Skipping remaining sessions"
            break
        elif [[ $REPLY =~ ^[Yy]$ ]]; then
            map_session_to_workspace "$uuid" "$project" "$timestamp"
        else
            echo "Skipped"
        fi

        echo ""
    done

    echo "Migration complete: $current of $total processed"
}

# Main command logic
main() {
    local days_filter=""
    local active_only=true  # Default: show only active sessions

    # Parse options
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --verbose)
                VERBOSE=true
                shift
                ;;
            --all)
                active_only=false  # Override default: show all sessions
                shift
                ;;
            --days)
                days_filter="${2:-}"
                shift 2
                ;;
            *)
                log_error "Unknown option: $1"
                return 2
                ;;
        esac
    done

    log_info "Discovering Claude sessions from history.jsonl..."

    # Step 1: Discover all Claude sessions
    local sessions
    if ! sessions=$(discover_claude_sessions); then
        log_error "Failed to discover Claude sessions"
        return 1
    fi

    if [[ -z "$sessions" ]]; then
        log_info "No Claude sessions found in history.jsonl"
        return 0
    fi

    # Step 2: Apply filters and deduplicate
    local cutoff_ts=""
    if [[ -n "$days_filter" ]]; then
        cutoff_ts=$(date -d "$days_filter days ago" +%s)
        log_debug "Filtering to sessions newer than $(date -d "@$cutoff_ts" '+%Y-%m-%d')"
    fi

    # Deduplicate sessions (keep most recent activity per UUID)
    local -A unique_sessions
    while IFS='|' read -r uuid project timestamp; do
        # Apply time filter
        if [[ -n "$cutoff_ts" ]]; then
            local session_ts=$((timestamp / 1000))
            if [[ $session_ts -lt $cutoff_ts ]]; then
                log_debug "Filtered out (too old): $uuid"
                continue
            fi
        fi

        # Apply active-only filter (>20 messages, >24h duration)
        if [[ "$active_only" == true ]]; then
            local stats
            stats=$(get_session_stats "$uuid")
            local msg_count="${stats%%|*}"
            local duration="${stats#*|}"
            duration="${duration%%|*}"

            # Filter: must have >20 messages AND >24h duration
            if [[ ${msg_count:-0} -le 20 ]] || [[ $(echo "$duration < 24" | bc -l 2>/dev/null || echo "1") -eq 1 ]]; then
                log_debug "Filtered out (not active enough): $uuid ($msg_count msgs, ${duration}h)"
                continue
            fi
        fi

        # Deduplicate: keep most recent timestamp per UUID
        if [[ -z "${unique_sessions[$uuid]:-}" ]] || [[ $timestamp -gt ${unique_sessions[$uuid]##*|} ]]; then
            unique_sessions[$uuid]="$project|$timestamp"
        fi
    done <<< "$sessions"

    # Step 3: Match deduplicated sessions to manifests
    local matched=0
    local orphaned=0
    local -a orphan_list=()

    for uuid in "${!unique_sessions[@]}"; do
        local data="${unique_sessions[$uuid]}"
        local project="${data%|*}"
        local timestamp="${data##*|}"

        # Try to find existing manifest
        local manifest
        manifest=$(find_manifest_by_claude_uuid "$uuid")

        if [[ -n "$manifest" ]]; then
            ((matched++)) || true
            log_debug "Matched: $uuid"
        else
            # Check if we can match by worktree path
            manifest=$(grep -l "path: $project" "${SESSIONS_DIR:-$HOME/sessions}"/*/manifest.yaml 2>/dev/null | head -1 || true)

            if [[ -n "$manifest" ]]; then
                ((matched++)) || true
                log_debug "Matched by path: $uuid"
            else
                ((orphaned++)) || true
                orphan_list+=("$uuid|$project|$timestamp")
                log_debug "Orphaned: $uuid"
            fi
        fi
    done

    # Step 4: Report findings
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Session Discovery Report"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Matched sessions: $matched"
    echo "Orphaned sessions: $orphaned"
    echo ""

    if [[ $orphaned -eq 0 ]]; then
        log_success "All sessions are mapped!"
        return 0
    fi

    # Step 5: Migrate orphaned sessions
    if [[ "$auto_mode" == true ]]; then
        log_warn "Auto mode not yet implemented - use interactive mode"
        return 1
    fi

    migrate_orphaned_sessions "${orphan_list[@]}"
}

# Parse --help before main
if [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
    show_help
    exit 0
fi

main "$@"
