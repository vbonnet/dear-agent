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
source "$LIB_DIR/tmux-utils.sh"
source "$LIB_DIR/recovery-utils.sh"

# Show help
show_help() {
    cat <<EOF
Usage: session resume <identifier>

Resume a Claude or workspace session by any identifier.

The identifier can be:
  - Tmux session name (e.g., claude-1)
  - Workspace session ID (e.g., github.com-user-repo-main)
  - Claude UUID (e.g., c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2)
  - Partial session ID (if unique match)

The command will:
  1. Resolve the identifier to find the session manifest
  2. Perform health checks (directories exist, no corruption)
  3. Create or attach to tmux session
  4. Auto-resume Claude with the correct UUID
  5. Update manifest timestamps

Options:
  -h, --help     Show this help message
  --verbose      Enable verbose output

Examples:
  session resume claude-1
  session resume github.com-user-repo-main
  session resume c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2

See also:
  session list    List all sessions
  session sync    Discover and sync Claude sessions
EOF
}

# resolve_session_identifier(identifier)
# Try to find manifest by various matching strategies
# Returns: Path to manifest.yaml or empty string
resolve_session_identifier() {
    local identifier="$1"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    log_debug "Resolving identifier: $identifier"

    # Try 1: Exact workspace session ID
    if [[ -d "$sessions_dir/$identifier" ]]; then
        local manifest="$sessions_dir/$identifier/manifest.yaml"
        if [[ -f "$manifest" ]]; then
            log_debug "Found by exact workspace ID"
            echo "$manifest"
            return 0
        fi
    fi

    # Try 2: Claude UUID
    local manifest
    manifest=$(find_manifest_by_claude_uuid "$identifier")
    if [[ -n "$manifest" ]]; then
        log_debug "Found by Claude UUID"
        echo "$manifest"
        return 0
    fi

    # Try 3: Tmux session name
    manifest=$(find_manifest_by_tmux_name "$identifier")
    if [[ -n "$manifest" ]]; then
        log_debug "Found by tmux name"
        echo "$manifest"
        return 0
    fi

    # Try 4: Partial match on session ID
    local matches=("$sessions_dir"/*"$identifier"*/manifest.yaml)
    if [[ ${#matches[@]} -eq 1 ]] && [[ -f "${matches[0]}" ]]; then
        log_debug "Found by partial match"
        echo "${matches[0]}"
        return 0
    elif [[ ${#matches[@]} -gt 1 ]]; then
        log_error "Ambiguous identifier '$identifier' matches multiple sessions:"
        for match in "${matches[@]}"; do
            local sid
            sid=$(basename "$(dirname "$match")")
            echo "  - $sid" >&2
        done
        return 0  # Return empty (handled by caller)
    fi

    # Not found
    log_debug "Identifier not found"
    return 0  # Return empty (handled by caller)
}

# handle_session_not_found(identifier)
# Offer to run sync to discover sessions
# Review Condition #2: Auto-sync offer on failure
handle_session_not_found() {
    local identifier="$1"

    log_error "Session not found: $identifier"
    echo ""
    echo "Possible reasons:"
    echo "  - Session hasn't been discovered yet"
    echo "  - Session was created outside workspace management"
    echo "  - Manifest is out of sync"
    echo ""

    # Review Condition #2: Offer auto-sync
    read -p "Run 'session sync' to discover sessions? (y/N): " -n 1 -r
    echo ""

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo ""
        log_info "Running session sync..."

        # Run sync command
        "$SCRIPT_DIR/sync.sh"

        # Retry resolution
        local manifest_path
        manifest_path=$(resolve_session_identifier "$identifier")
        if [[ -z "$manifest_path" ]]; then
            echo ""
            log_error "Session still not found after sync"
            echo ""
            echo "The session may not exist, or may not be in a recognized location."
            echo "Check:"
            echo "  - ~/.claude/history.jsonl contains the session"
            echo "  - Working directory is accessible"
            return 3
        fi

        # Found after sync! Return the manifest path via stdout
        echo "$manifest_path"
        return 0
    else
        echo ""
        echo "You can run manually: session sync"
        return 3
    fi
}

# perform_health_checks(claude_uuid, worktree_path, manifest_path, session_id)
# Validate session health before resuming
# Review Conditions #3, #8
perform_health_checks() {
    local claude_uuid="$1"
    local worktree_path="$2"
    local manifest_path="$3"
    local session_id="$4"

    log_debug "Performing health checks..."

    # Check 0: Detect corruption (Review Condition #8)
    if ! detect_session_corruption "$manifest_path"; then
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Session Corruption Detected"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "Recovery options:"
        echo "  1. Attempt automatic repair"
        echo "  2. Archive corrupted session"
        echo "  3. Cancel"
        echo ""
        read -p "Select option (1-3): " -n 1 -r choice
        echo ""
        echo ""

        case "$choice" in
            1)
                if repair_session_corruption "$manifest_path"; then
                    log_success "Corruption repaired, continuing..."
                else
                    log_error "Unable to repair automatically"
                    return 1
                fi
                ;;
            2)
                archive_session "$manifest_path" "$session_id" "corrupted"
                return 1
                ;;
            3)
                log_info "Cancelled by user"
                return 1
                ;;
            *)
                log_error "Invalid choice"
                return 1
                ;;
        esac
    fi

    # Check 1: Validate Claude session directories (Condition #3)
    if ! validate_claude_session_dirs "$claude_uuid"; then
        log_error "Claude session directories invalid or missing"
        echo ""
        echo "Recovery options:"
        echo "  1. Session may be corrupted - archive this manifest"
        echo "  2. Continue anyway (may fail)"
        echo ""
        read -p "Select option (1-2): " -n 1 -r choice
        echo ""

        case "$choice" in
            1)
                archive_session "$manifest_path" "$session_id" "invalid_claude_dirs"
                return 1
                ;;
            2)
                log_warn "Continuing without valid Claude directories..."
                ;;
            *)
                log_error "Invalid choice"
                return 1
                ;;
        esac
    fi

    # Check 2: Validate worktree exists
    if [[ ! -d "$worktree_path" ]]; then
        log_warn "Worktree directory not found: $worktree_path"
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "CWD Deleted Bug Detected"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "The worktree directory was deleted (common after machine restart)."
        echo ""
        echo "Recovery options:"
        echo "  1. Recreate worktree (if still in git worktree list)"
        echo "  2. Use fallback directory (~/sessions/$session_id/working)"
        echo "  3. Archive session and start fresh"
        echo "  4. Cancel"
        echo ""
        read -p "Select option (1-4): " -n 1 -r choice
        echo ""
        echo ""

        case "$choice" in
            1)
                if recover_deleted_worktree "$manifest_path" "$session_id" "$worktree_path"; then
                    log_success "Worktree recovered successfully!"
                else
                    log_error "Worktree recovery failed"
                    return 1
                fi
                ;;
            2)
                local fallback_path
                if fallback_path=$(use_fallback_directory "$manifest_path" "$session_id"); then
                    log_success "Using fallback directory: $fallback_path"
                    # Update worktree_path for resume
                    worktree_path="$fallback_path"
                else
                    log_error "Failed to create fallback directory"
                    return 1
                fi
                ;;
            3)
                archive_session "$manifest_path" "$session_id" "cwd_deleted"
                return 1
                ;;
            4)
                log_info "Cancelled by user"
                return 1
                ;;
            *)
                log_error "Invalid choice"
                return 1
                ;;
        esac
    fi

    log_debug "Health checks passed"
    return 0
}

# Main command logic
main() {
    local identifier="${1:-}"

    if [[ -z "$identifier" ]]; then
        log_error "Session identifier required"
        echo ""
        echo "Usage: session resume <identifier>"
        echo ""
        echo "Identifier can be:"
        echo "  - Tmux session name (e.g., claude-1)"
        echo "  - Workspace session ID (e.g., github.com-user-repo-main)"
        echo "  - Claude UUID (e.g., c86ffd41-cbcc-4bfa-8b1f-...)"
        return 2
    fi

    log_info "Resuming session: $identifier"

    # Step 1: Resolve identifier → manifest
    local manifest_path
    manifest_path=$(resolve_session_identifier "$identifier")

    if [[ -z "$manifest_path" ]]; then
        # Offer auto-sync (Review Condition #2)
        manifest_path=$(handle_session_not_found "$identifier")
        if [[ $? -ne 0 ]] || [[ -z "$manifest_path" ]]; then
            return 3
        fi
    fi

    log_debug "Found manifest: $manifest_path"

    # Step 2: Read manifest
    local session_id
    local worktree_path
    local claude_uuid
    local tmux_name

    session_id=$(basename "$(dirname "$manifest_path")")
    worktree_path=$(read_manifest_nested "$manifest_path" "worktree" "path")
    claude_uuid=$(read_claude_session_id "$manifest_path")
    tmux_name=$(read_tmux_session_name "$manifest_path")

    # If no Claude session, this is workspace-only
    if [[ -z "$claude_uuid" ]]; then
        log_info "No Claude session associated, resuming workspace only"
        log_warn "Workspace-only resume not yet implemented"
        return 0
    fi

    # If no tmux name yet, generate one
    if [[ -z "$tmux_name" ]]; then
        log_debug "No tmux name in manifest, generating..."
        tmux_name="claude-1"  # TODO: smarter naming

        # Update manifest with tmux name
        update_tmux_metadata "$manifest_path" "$tmux_name"
    fi

    log_info "Session ID: $session_id"
    log_info "Claude UUID: $claude_uuid"
    log_info "Tmux name: $tmux_name"
    log_info "Worktree: $worktree_path"

    # Step 3: Health checks
    if ! perform_health_checks "$claude_uuid" "$worktree_path" "$manifest_path" "$session_id"; then
        return $?
    fi

    # Step 4: Update last_activity timestamp
    local now
    now=$(date -Iseconds)
    local started_at
    started_at=$(read_manifest_nested "$manifest_path" "claude" "started_at")
    if [[ -z "$started_at" ]]; then
        started_at="$now"
    fi

    update_claude_metadata "$manifest_path" "$claude_uuid" "$started_at" "$now"

    # Step 5: Ensure tmux session and resume
    # Note: This function does NOT return - it execs tmux attach
    ensure_tmux_and_resume "$tmux_name" "$worktree_path" "$claude_uuid"
}

# Parse --help before main
if [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
    show_help
    exit 0
fi

main "$@"
