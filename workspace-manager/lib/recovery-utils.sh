#!/bin/bash

# recovery-utils.sh - Session recovery and cleanup utilities
# Part of unified session management CLI
# Handles edge cases: CWD deleted bug, corruption, cleanup

# recover_deleted_worktree(manifest_path, session_id, worktree_path)
# Attempt to recreate a deleted worktree directory
# Returns: 0 if recovery successful, 1 if failed
recover_deleted_worktree() {
    local manifest_path="$1"
    local session_id="$2"
    local worktree_path="$3"

    log_info "Attempting worktree recovery..."

    # Read repository info from manifest
    local repo_url
    local branch
    repo_url=$(read_manifest_nested "$manifest_path" "repository" "url")
    branch=$(read_manifest_nested "$manifest_path" "worktree" "branch")

    if [[ -z "$repo_url" ]]; then
        log_error "No repository URL in manifest - cannot recreate worktree"
        return 1
    fi

    # Check if worktree is still registered in git
    local parent_repo
    parent_repo=$(dirname "$worktree_path")

    if [[ -d "$parent_repo/.git" ]]; then
        log_info "Found parent repository: $parent_repo"

        # Check git worktree list
        if git -C "$parent_repo" worktree list 2>/dev/null | grep -q "$worktree_path"; then
            log_info "Worktree still registered in git, attempting repair..."

            # Try to recreate the directory
            mkdir -p "$worktree_path"

            # Git should automatically repair the worktree
            if git -C "$parent_repo" worktree repair 2>/dev/null; then
                log_success "Worktree repaired successfully!"
                return 0
            else
                log_warn "Git worktree repair failed"
            fi
        else
            log_info "Worktree not in git worktree list"

            # Offer to re-add worktree
            if [[ -n "$branch" ]]; then
                log_info "Attempting to re-add worktree for branch: $branch"

                if git -C "$parent_repo" worktree add "$worktree_path" "$branch" 2>/dev/null; then
                    log_success "Worktree re-added successfully!"
                    return 0
                else
                    log_warn "Failed to re-add worktree"
                fi
            fi
        fi
    fi

    log_error "Could not automatically recover worktree"
    log_info "Manual recovery required:"
    log_info "  1. Clone repository: git clone $repo_url"
    log_info "  2. Add worktree: git worktree add $worktree_path $branch"
    log_info "  3. Retry session resume"

    return 1
}

# use_fallback_directory(manifest_path, session_id)
# Create and use fallback directory when worktree is missing
# Returns: Path to fallback directory
use_fallback_directory() {
    local manifest_path="$1"
    local session_id="$2"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"

    local fallback_dir="$sessions_dir/$session_id/working"

    log_info "Creating fallback directory: $fallback_dir"

    # Create fallback directory
    if ! mkdir -p "$fallback_dir"; then
        log_error "Failed to create fallback directory"
        return 1
    fi

    # Update manifest with new worktree path
    log_info "Updating manifest with fallback path..."

    # Use sed to update worktree path
    if sed -i "s|^  path:.*|  path: $fallback_dir|" "$manifest_path"; then
        log_success "Manifest updated with fallback path"
        echo "$fallback_dir"
        return 0
    else
        log_error "Failed to update manifest"
        return 1
    fi
}

# archive_session(manifest_path, session_id, reason)
# Archive a session that can't be recovered
# Moves manifest to archive directory with metadata
archive_session() {
    local manifest_path="$1"
    local session_id="$2"
    local reason="${3:-user_requested}"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"
    local archive_dir="$sessions_dir/.archive"

    log_info "Archiving session: $session_id"
    log_info "Reason: $reason"

    # Create archive directory
    mkdir -p "$archive_dir"

    # Create timestamped archive subdirectory
    local timestamp
    timestamp=$(date +%Y%m%d-%H%M%S)
    local archive_session_dir="$archive_dir/${session_id}-${timestamp}"

    if ! mkdir -p "$archive_session_dir"; then
        log_error "Failed to create archive directory"
        return 1
    fi

    # Move entire session directory
    local session_dir
    session_dir=$(dirname "$manifest_path")

    log_debug "Moving $session_dir to $archive_session_dir"

    if mv "$session_dir"/* "$archive_session_dir/" 2>/dev/null; then
        # Create archive metadata
        cat > "$archive_session_dir/.archive-metadata" <<EOF
archived_at: $(date -Iseconds)
original_session_id: $session_id
archive_reason: $reason
archived_by: $(whoami)
EOF

        # Remove empty session directory
        rmdir "$session_dir" 2>/dev/null

        log_success "Session archived to: $archive_session_dir"
        log_info "To restore: mv $archive_session_dir $session_dir"
        return 0
    else
        log_error "Failed to move session to archive"
        return 1
    fi
}

# detect_session_corruption(manifest_path)
# Review Condition #8: Detect corrupted session data
# Returns: 0 if healthy, 1 if corrupted
detect_session_corruption() {
    local manifest_path="$1"
    local corruption_found=false
    local issues=()

    log_debug "Checking for session corruption..."

    # Check 1: Manifest is valid YAML
    if ! grep -q "^session_id:" "$manifest_path"; then
        issues+=("Manifest missing session_id field")
        corruption_found=true
    fi

    # Check 2: Timestamps are valid ISO format
    local created_at
    created_at=$(read_manifest_field "$manifest_path" "created_at")
    if [[ -n "$created_at" ]] && ! date -d "$created_at" &>/dev/null; then
        issues+=("Invalid created_at timestamp: $created_at")
        corruption_found=true
    fi

    # Check 3: Claude UUID format (if present)
    local claude_uuid
    claude_uuid=$(read_claude_session_id "$manifest_path")
    if [[ -n "$claude_uuid" ]] && [[ ! "$claude_uuid" =~ ^[a-f0-9-]{36}$ ]]; then
        issues+=("Invalid Claude UUID format: $claude_uuid")
        corruption_found=true
    fi

    # Check 4: Worktree path is absolute (if present)
    local worktree_path
    worktree_path=$(read_manifest_nested "$manifest_path" "worktree" "path")
    if [[ -n "$worktree_path" ]] && [[ "$worktree_path" != /* ]]; then
        issues+=("Worktree path is not absolute: $worktree_path")
        corruption_found=true
    fi

    # Check 5: Claude session directories match UUID
    if [[ -n "$claude_uuid" ]]; then
        local session_env="$HOME/.claude/session-env/$claude_uuid"
        local file_history="$HOME/.claude/file-history/$claude_uuid"

        # Check paths in manifest match actual UUID
        local manifest_session_env
        manifest_session_env=$(read_manifest_nested "$manifest_path" "claude" "session_env_path")

        if [[ -n "$manifest_session_env" ]] && [[ "$manifest_session_env" != "$session_env" ]]; then
            issues+=("Claude session_env_path mismatch")
            corruption_found=true
        fi
    fi

    # Report findings
    if [[ "$corruption_found" == true ]]; then
        log_warn "Session corruption detected:"
        for issue in "${issues[@]}"; do
            echo "  - $issue" >&2
        done
        return 1
    else
        log_debug "No corruption detected"
        return 0
    fi
}

# repair_session_corruption(manifest_path)
# Review Condition #8: Attempt to repair corrupted session
# Returns: 0 if repaired, 1 if unable to repair
repair_session_corruption() {
    local manifest_path="$1"
    local repaired=false

    log_info "Attempting to repair session corruption..."

    # Create backup before repairs
    local backup_path="${manifest_path}.backup.$(date +%s)"
    cp "$manifest_path" "$backup_path"
    log_debug "Created backup: $backup_path"

    # Repair 1: Fix missing session_id
    if ! grep -q "^session_id:" "$manifest_path"; then
        local session_dir
        session_dir=$(basename "$(dirname "$manifest_path")")

        # Insert session_id at top of file
        sed -i "1i session_id: $session_dir" "$manifest_path"
        log_info "Repaired: Added missing session_id"
        repaired=true
    fi

    # Repair 2: Fix invalid timestamps
    local created_at
    created_at=$(read_manifest_field "$manifest_path" "created_at")
    if [[ -n "$created_at" ]] && ! date -d "$created_at" &>/dev/null; then
        local now
        now=$(date -Iseconds)
        sed -i "s|^created_at:.*|created_at: $now|" "$manifest_path"
        log_info "Repaired: Fixed invalid created_at timestamp"
        repaired=true
    fi

    # Repair 3: Fix Claude session paths
    local claude_uuid
    claude_uuid=$(read_claude_session_id "$manifest_path")
    if [[ -n "$claude_uuid" ]]; then
        local expected_session_env="$HOME/.claude/session-env/$claude_uuid"
        local expected_file_history="$HOME/.claude/file-history/$claude_uuid"

        # Update paths if wrong
        sed -i "s|^  session_env_path:.*|  session_env_path: $expected_session_env|" "$manifest_path"
        sed -i "s|^  file_history_path:.*|  file_history_path: $expected_file_history|" "$manifest_path"
        log_info "Repaired: Fixed Claude session paths"
        repaired=true
    fi

    if [[ "$repaired" == true ]]; then
        log_success "Session repaired successfully"
        log_info "Backup saved to: $backup_path"
        return 0
    else
        log_warn "No repairs performed - manual intervention required"
        rm "$backup_path"  # Remove unnecessary backup
        return 1
    fi
}

# cleanup_stale_sessions(days_threshold)
# Clean up sessions with no activity for N days
# Returns: Number of sessions archived
cleanup_stale_sessions() {
    local days="${1:-30}"
    local sessions_dir="${SESSIONS_DIR:-$HOME/sessions}"
    local threshold_ts
    threshold_ts=$(date -d "$days days ago" +%s)
    local count=0

    log_info "Finding sessions with no activity for $days+ days..."

    for manifest in "$sessions_dir"/*/manifest.yaml; do
        if [[ ! -f "$manifest" ]]; then
            continue
        fi

        local last_activity
        last_activity=$(read_manifest_field "$manifest" "last_activity")

        if [[ -z "$last_activity" ]]; then
            continue  # Skip if no timestamp
        fi

        # Convert to Unix timestamp
        local activity_ts
        activity_ts=$(date -d "$last_activity" +%s 2>/dev/null)

        if [[ -z "$activity_ts" ]]; then
            continue  # Skip if invalid timestamp
        fi

        # Check if stale
        if [[ $activity_ts -lt $threshold_ts ]]; then
            local session_id
            session_id=$(basename "$(dirname "$manifest")")

            log_info "Stale session found: $session_id (last activity: $last_activity)"

            read -p "Archive this session? (y/N): " -n 1 -r
            echo ""

            if [[ $REPLY =~ ^[Yy]$ ]]; then
                if archive_session "$manifest" "$session_id" "stale_${days}d"; then
                    ((count++)) || true  # Prevent exit on count=0 with set -e
                fi
            fi
        fi
    done

    log_info "Archived $count stale sessions"
    return "$count"
}

# Export functions
export -f recover_deleted_worktree
export -f use_fallback_directory
export -f archive_session
export -f detect_session_corruption
export -f repair_session_corruption
export -f cleanup_stale_sessions
