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
source "$LIB_DIR/recovery-utils.sh"

# Show help
show_help() {
    cat <<EOF
Usage: session cleanup [options]

Clean up stale sessions, archived sessions, and orphaned data.

Operations:
  --stale <days>       Archive sessions inactive for N days (default: 30)
  --archive-list       List archived sessions
  --archive-restore ID Restore an archived session
  --archive-purge      Permanently delete archived sessions
  --orphaned-tmux      List tmux sessions not in manifest
  --orphaned-claude    List Claude sessions not in manifest
  -h, --help           Show this help message
  --verbose            Enable verbose output

Examples:
  session cleanup --stale 30
  session cleanup --archive-list
  session cleanup --archive-restore my-session-20251203-123456
  session cleanup --orphaned-tmux

See also:
  session list    List all sessions
  session sync    Discover and sync Claude sessions
EOF
}

# list_archived_sessions()
# Display all archived sessions with metadata
list_archived_sessions() {
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"
    local archive_dir="$sessions_dir/.archive"

    if [[ ! -d "$archive_dir" ]]; then
        log_info "No archived sessions found"
        return 0
    fi

    echo ""
    echo "Archived Sessions"
    echo ""
    printf "%-40s | %-15s | %s\n" "Archive ID" "Original ID" "Archived At"
    echo "────────────────────────────────────────────────────────────────────────────────"

    local count=0

    for archive in "$archive_dir"/*/; do
        if [[ ! -d "$archive" ]]; then
            continue
        fi

        local archive_id
        archive_id=$(basename "$archive")

        local metadata="$archive/.archive-metadata"
        if [[ ! -f "$metadata" ]]; then
            continue
        fi

        # Read metadata
        local original_id
        local archived_at
        local reason

        original_id=$(grep "^original_session_id:" "$metadata" | awk '{print $2}')
        archived_at=$(grep "^archived_at:" "$metadata" | awk '{print $2}')
        reason=$(grep "^archive_reason:" "$metadata" | awk '{print $2}')

        # Format timestamp
        local timestamp
        timestamp=$(date -d "$archived_at" '+%Y-%m-%d %H:%M' 2>/dev/null || echo "$archived_at")

        printf "%-40s | %-15s | %s (%s)\n" \
            "$archive_id" \
            "$original_id" \
            "$timestamp" \
            "$reason"

        ((count++)) || true  # Prevent exit on count=0 with set -e
    done

    echo ""
    echo "Total: $count archived sessions"
    echo ""
}

# restore_archived_session(archive_id)
# Restore an archived session back to active sessions
restore_archived_session() {
    local archive_id="$1"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"
    local archive_dir="$sessions_dir/.archive"
    local archive_path="$archive_dir/$archive_id"

    if [[ ! -d "$archive_path" ]]; then
        log_error "Archive not found: $archive_id"
        return 1
    fi

    # Read original session ID
    local metadata="$archive_path/.archive-metadata"
    if [[ ! -f "$metadata" ]]; then
        log_error "Archive metadata missing"
        return 1
    fi

    local original_id
    original_id=$(grep "^original_session_id:" "$metadata" | awk '{print $2}')

    log_info "Restoring session: $original_id"

    # Check if session ID already exists
    if [[ -d "$sessions_dir/$original_id" ]]; then
        log_error "Session ID already exists: $original_id"
        log_info "Choose a different ID or remove the existing session first"
        return 1
    fi

    # Restore session
    mkdir -p "$sessions_dir/$original_id"

    if cp -r "$archive_path"/* "$sessions_dir/$original_id/" 2>/dev/null; then
        # Remove metadata file from restored session
        rm -f "$sessions_dir/$original_id/.archive-metadata"

        log_success "Session restored to: $sessions_dir/$original_id"

        # Ask if should delete archive
        read -p "Delete archive? (y/N): " -n 1 -r
        echo ""

        if [[ $REPLY =~ ^[Yy]$ ]]; then
            rm -rf "$archive_path"
            log_success "Archive deleted"
        fi

        return 0
    else
        log_error "Failed to restore session"
        return 1
    fi
}

# purge_archived_sessions()
# Permanently delete all archived sessions
purge_archived_sessions() {
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"
    local archive_dir="$sessions_dir/.archive"

    if [[ ! -d "$archive_dir" ]]; then
        log_info "No archived sessions to purge"
        return 0
    fi

    # Count archives
    local count
    count=$(find "$archive_dir" -mindepth 1 -maxdepth 1 -type d | wc -l)

    if [[ $count -eq 0 ]]; then
        log_info "No archived sessions to purge"
        return 0
    fi

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "WARNING: Permanent Deletion"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "This will PERMANENTLY DELETE $count archived sessions."
    echo "This action CANNOT be undone."
    echo ""
    read -p "Are you sure? Type 'DELETE' to confirm: " confirm

    if [[ "$confirm" != "DELETE" ]]; then
        log_info "Purge cancelled"
        return 0
    fi

    log_info "Purging $count archived sessions..."

    if rm -rf "$archive_dir"; then
        log_success "All archived sessions deleted permanently"
        return 0
    else
        log_error "Failed to purge archives"
        return 1
    fi
}

# list_orphaned_tmux_sessions()
# Find tmux sessions not tracked in manifests
list_orphaned_tmux_sessions() {
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    if ! command -v tmux &>/dev/null; then
        log_error "tmux not installed"
        return 1
    fi

    # Get all tmux sessions
    local tmux_sessions
    if ! tmux_sessions=$(tmux list-sessions -F '#{session_name}' 2>/dev/null); then
        log_info "No tmux sessions running"
        return 0
    fi

    echo ""
    echo "Orphaned Tmux Sessions (not in manifests)"
    echo ""

    local orphan_count=0

    while IFS= read -r tmux_name; do
        # Check if this tmux session is in any manifest
        if ! grep -r "session_name: $tmux_name" "$sessions_dir"/*/manifest.yaml &>/dev/null; then
            echo "  - $tmux_name"
            ((orphan_count++)) || true  # Prevent exit on count=0 with set -e
        fi
    done <<< "$tmux_sessions"

    if [[ $orphan_count -eq 0 ]]; then
        echo "  (none)"
    fi

    echo ""
    echo "Total: $orphan_count orphaned tmux sessions"
    echo ""
}

# list_orphaned_claude_sessions()
# Find Claude sessions in history.jsonl not tracked in manifests
list_orphaned_claude_sessions() {
    local history_file="${CLAUDE_HISTORY:-$HOME/.claude/history.jsonl}"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    if [[ ! -f "$history_file" ]]; then
        log_error "Claude history.jsonl not found"
        return 1
    fi

    log_info "Scanning Claude history for orphaned sessions..."

    # Use claude-discovery to parse history
    source "$LIB_DIR/claude-discovery.sh"

    local sessions
    if ! sessions=$(discover_claude_sessions); then
        log_error "Failed to discover Claude sessions"
        return 1
    fi

    echo ""
    echo "Orphaned Claude Sessions (not in manifests)"
    echo ""

    local orphan_count=0

    while IFS='|' read -r uuid project timestamp; do
        # Check if this UUID is in any manifest
        if ! grep -r "session_id: $uuid" "$sessions_dir"/*/manifest.yaml &>/dev/null; then
            echo "  - $uuid (project: $project)"
            ((orphan_count++)) || true  # Prevent exit on count=0 with set -e
        fi
    done <<< "$sessions"

    if [[ $orphan_count -eq 0 ]]; then
        echo "  (none)"
    fi

    echo ""
    echo "Total: $orphan_count orphaned Claude sessions"
    echo ""
    echo "Run 'session sync' to create manifests for these sessions"
    echo ""
}

# Main command logic
main() {
    local operation=""
    local days=30
    local archive_id=""

    # Parse options
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --stale)
                operation="stale"
                if [[ -n "${2:-}" ]] && [[ "$2" =~ ^[0-9]+$ ]]; then
                    days="$2"
                    shift
                fi
                shift
                ;;
            --archive-list)
                operation="archive-list"
                shift
                ;;
            --archive-restore)
                operation="archive-restore"
                archive_id="${2:-}"
                shift 2
                ;;
            --archive-purge)
                operation="archive-purge"
                shift
                ;;
            --orphaned-tmux)
                operation="orphaned-tmux"
                shift
                ;;
            --orphaned-claude)
                operation="orphaned-claude"
                shift
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                return 2
                ;;
        esac
    done

    # Execute operation
    case "$operation" in
        stale)
            cleanup_stale_sessions "$days"
            ;;
        archive-list)
            list_archived_sessions
            ;;
        archive-restore)
            if [[ -z "$archive_id" ]]; then
                log_error "Archive ID required"
                echo "Usage: session cleanup --archive-restore <archive-id>"
                return 2
            fi
            restore_archived_session "$archive_id"
            ;;
        archive-purge)
            purge_archived_sessions
            ;;
        orphaned-tmux)
            list_orphaned_tmux_sessions
            ;;
        orphaned-claude)
            list_orphaned_claude_sessions
            ;;
        *)
            log_error "No operation specified"
            show_help
            return 2
            ;;
    esac
}

# Parse --help before main
if [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
    show_help
    exit 0
fi

main "$@"
