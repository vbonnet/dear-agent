#!/usr/bin/env bash
#
# manifest-utils.sh - Session manifest YAML manipulation utilities
#
# Provides functions for reading, writing, and updating session manifest files
# Includes R2.2 auto-update mechanism for keeping manifests current
#
# Usage:
#   source lib/manifest-utils.sh
#
# Dependencies:
#   lib/common-utils.sh

# Source guard to prevent double-sourcing
[[ -n "${MANIFEST_UTILS_LOADED:-}" ]] && return 0
readonly MANIFEST_UTILS_LOADED=1

set -euo pipefail

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common-utils.sh"

#######################################
# YAML Reading Functions
#######################################

# Read a top-level field from a YAML manifest
# Arguments:
#   $1 - Manifest file path
#   $2 - Field name
# Outputs:
#   Field value, or empty string if not found
# Returns:
#   0 on success
read_manifest_field() {
  local manifest="$1"
  local field="$2"

  validate_file "$manifest" "Manifest"

  log_debug "Reading manifest field: $field from $manifest"

  # Use grep + sed to extract field value
  local value
  value=$(grep "^${field}:" "$manifest" | head -n1 | sed "s/^${field}:[[:space:]]*//" || echo "")

  echo "$value"

  return 0
}

# Read a nested field from a YAML manifest
# Arguments:
#   $1 - Manifest file path
#   $2 - Parent field name
#   $3 - Nested field name
# Outputs:
#   Field value, or empty string if not found
# Returns:
#   0 on success
read_manifest_nested() {
  local manifest="$1"
  local parent="$2"
  local field="$3"

  validate_file "$manifest" "Manifest"

  log_debug "Reading nested manifest field: $parent.$field from $manifest"

  # Extract nested field (assumes 2-space indentation)
  local value
  value=$(awk "
    /^${parent}:/ { in_section=1; next }
    in_section && /^[^ ]/ { in_section=0 }
    in_section && /^  ${field}:/ { sub(/^  ${field}:[[:space:]]*/, \"\"); print; exit }
  " "$manifest" || echo "")

  echo "$value"

  return 0
}

#######################################
# YAML Writing Functions
#######################################

# Update a top-level field in a YAML manifest (atomic operation)
# Arguments:
#   $1 - Manifest file path
#   $2 - Field name
#   $3 - New value
# Returns:
#   0 on success, 1 on error
update_manifest_field() {
  local manifest="$1"
  local field="$2"
  local value="$3"

  validate_file "$manifest" "Manifest"

  log_debug "Updating manifest field: $field = $value in $manifest"

  # Create temp file for atomic update
  local temp_file="${manifest}.tmp.$$"

  # Update field using sed
  if grep -q "^${field}:" "$manifest"; then
    # Field exists, update it
    sed "s|^${field}:.*|${field}: ${value}|" "$manifest" > "$temp_file"
  else
    # Field doesn't exist, append it
    cat "$manifest" > "$temp_file"
    echo "${field}: ${value}" >> "$temp_file"
  fi

  # Atomic move
  if mv "$temp_file" "$manifest"; then
    log_debug "Successfully updated $field in manifest"
    return 0
  else
    log_error "Failed to update manifest file"
    rm -f "$temp_file"
    return 1
  fi
}

# Update a nested field in a YAML manifest (atomic operation)
# Arguments:
#   $1 - Manifest file path
#   $2 - Parent field name
#   $3 - Nested field name
#   $4 - New value
# Returns:
#   0 on success, 1 on error
update_manifest_nested_field() {
  local manifest="$1"
  local parent="$2"
  local field="$3"
  local value="$4"

  validate_file "$manifest" "Manifest"

  log_debug "Updating nested manifest field: $parent.$field = $value in $manifest"

  # Create temp file for atomic update
  local temp_file="${manifest}.tmp.$$"

  # Update nested field using awk
  awk -v parent="$parent" -v field="$field" -v value="$value" '
    BEGIN { in_section=0; updated=0 }
    /^[^ ]/ && $0 !~ "^" parent ":" { in_section=0 }
    $0 ~ "^" parent ":" { in_section=1; print; next }
    in_section && $0 ~ "^  " field ":" {
      print "  " field ": " value
      updated=1
      next
    }
    { print }
    END {
      if (!updated && in_section) {
        print "  " field ": " value
      }
    }
  ' "$manifest" > "$temp_file"

  # Atomic move
  if mv "$temp_file" "$manifest"; then
    log_debug "Successfully updated $parent.$field in manifest"
    return 0
  else
    log_error "Failed to update manifest file"
    rm -f "$temp_file"
    return 1
  fi
}

#######################################
# R2.2 Auto-Update Functions
#######################################

# Update the last_activity timestamp in manifest
# Arguments:
#   $1 - Manifest file path
# Returns:
#   0 on success, gracefully continues on error
update_manifest_activity() {
  local manifest="$1"

  # Check if manifest exists
  if [[ ! -f "$manifest" ]]; then
    log_debug "Manifest not found, skipping activity update: $manifest"
    return 0
  fi

  # Generate current timestamp (ISO 8601)
  local timestamp
  timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)

  log_debug "Updating last_activity to $timestamp"

  # Update field (graceful degradation on error)
  if ! update_manifest_field "$manifest" "last_activity" "$timestamp"; then
    log_warn "Failed to update last_activity in manifest (continuing)"
  fi

  return 0
}

# Add an artifact to the manifest artifacts list
# Arguments:
#   $1 - Manifest file path
#   $2 - Artifact filename
# Returns:
#   0 on success, gracefully continues on error
add_manifest_artifact() {
  local manifest="$1"
  local artifact="$2"

  # Check if manifest exists
  if [[ ! -f "$manifest" ]]; then
    log_debug "Manifest not found, skipping artifact addition: $manifest"
    return 0
  fi

  log_debug "Adding artifact to manifest: $artifact"

  # Create temp file
  local temp_file="${manifest}.tmp.$$"

  # Check if artifact already exists in list
  if grep -q "^  - ${artifact}$" "$manifest"; then
    log_debug "Artifact already in manifest: $artifact"
    return 0
  fi

  # Add artifact to list using awk
  awk -v artifact="$artifact" '
    BEGIN { in_artifacts=0; added=0 }
    /^artifacts:/ { in_artifacts=1; print; next }
    in_artifacts && /^[^ ]/ {
      if (!added) {
        print "  - " artifact
        added=1
      }
      in_artifacts=0
    }
    { print }
    END {
      if (in_artifacts && !added) {
        print "  - " artifact
      }
    }
  ' "$manifest" > "$temp_file"

  # Atomic move (graceful degradation on error)
  if mv "$temp_file" "$manifest"; then
    log_debug "Successfully added artifact to manifest"
  else
    log_warn "Failed to add artifact to manifest (continuing)"
    rm -f "$temp_file"
  fi

  return 0
}

# Update worktree-related fields in manifest
# Arguments:
#   $1 - Manifest file path
#   $2 - Worktree field name (e.g., "branch", "path", "commit")
#   $3 - New value
# Returns:
#   0 on success, gracefully continues on error
update_manifest_worktree_field() {
  local manifest="$1"
  local field="$2"
  local value="$3"

  # Check if manifest exists
  if [[ ! -f "$manifest" ]]; then
    log_debug "Manifest not found, skipping worktree field update: $manifest"
    return 0
  fi

  log_debug "Updating worktree.$field to $value"

  # Update nested field (graceful degradation on error)
  if ! update_manifest_nested_field "$manifest" "worktree" "$field" "$value"; then
    log_warn "Failed to update worktree.$field in manifest (continuing)"
  fi

  return 0
}

#######################################
# Manifest Creation
#######################################

# Create a new session manifest file
# Arguments:
#   $1 - Manifest file path
#   $2 - Session ID
#   $3 - Git URL
#   $4 - Repository path
#   $5 - Worktree path
#   $6 - Branch name
# Returns:
#   0 on success, 1 on error
create_manifest() {
  local manifest="$1"
  local session_id="$2"
  local git_url="$3"
  local repo_path="$4"
  local worktree_path="$5"
  local branch="$6"

  log_debug "Creating manifest: $manifest"

  # Validate inputs
  validate_not_empty "$session_id" "Session ID"
  validate_not_empty "$git_url" "Git URL"
  validate_not_empty "$repo_path" "Repository path"
  validate_not_empty "$worktree_path" "Worktree path"
  validate_not_empty "$branch" "Branch name"

  # Generate timestamps
  local now
  now=$(date -u +%Y-%m-%dT%H:%M:%SZ)

  # Get current commit (if in worktree)
  local current_commit="unknown"
  if [[ -d "$worktree_path/.git" ]]; then
    current_commit=$(cd "$worktree_path" && git rev-parse HEAD 2>/dev/null || echo "unknown")
  fi

  # Create manifest content
  cat > "$manifest" << EOF
# Session Manifest
# Generated: $now

session_id: $session_id
created_at: $now
last_activity: $now

repository:
  url: $git_url
  path: $repo_path

worktree:
  path: $worktree_path
  branch: $branch
  commit: $current_commit

artifacts: []

status: active
EOF

  if [[ -f "$manifest" ]]; then
    log_success "Created manifest: $manifest"
    return 0
  else
    log_error "Failed to create manifest"
    return 1
  fi
}

#######################################
# Manifest Display
#######################################

# Display a manifest in human-readable format
# Arguments:
#   $1 - Manifest file path
# Outputs:
#   Formatted manifest information
# Returns:
#   0 on success
display_manifest() {
  local manifest="$1"

  validate_file "$manifest" "Manifest"

  log_debug "Displaying manifest: $manifest"

  # Read key fields
  local session_id created_at last_activity status
  local repo_url repo_path worktree_path worktree_branch

  session_id=$(read_manifest_field "$manifest" "session_id")
  created_at=$(read_manifest_field "$manifest" "created_at")
  last_activity=$(read_manifest_field "$manifest" "last_activity")
  status=$(read_manifest_field "$manifest" "status")

  repo_url=$(read_manifest_nested "$manifest" "repository" "url")
  repo_path=$(read_manifest_nested "$manifest" "repository" "path")

  worktree_path=$(read_manifest_nested "$manifest" "worktree" "path")
  worktree_branch=$(read_manifest_nested "$manifest" "worktree" "branch")

  # Format and display
  echo "Session: $session_id"
  echo "Status: $status"
  echo ""
  echo "Created: $created_at"
  echo "Last Activity: $last_activity"
  echo ""
  echo "Repository:"
  echo "  URL: $repo_url"
  echo "  Path: $repo_path"
  echo ""
  echo "Worktree:"
  echo "  Path: $worktree_path"
  echo "  Branch: $worktree_branch"
  echo ""

  # Display artifacts if any
  local artifacts_count
  artifacts_count=$(grep -c "^  - " "$manifest" || echo "0")

  if [[ $artifacts_count -gt 0 ]]; then
    echo "Artifacts: ($artifacts_count)"
    grep "^  - " "$manifest" | sed 's/^  - /  - /'
  else
    echo "Artifacts: none"
  fi

  return 0
}

# List all manifests in a directory
# Arguments:
#   $1 - Directory to search (e.g., ~/sessions)
# Outputs:
#   List of manifest paths
# Returns:
#   0 on success
list_manifests() {
  local search_dir="$1"

  validate_directory "$search_dir" "Sessions directory"

  log_debug "Listing manifests in: $search_dir"

  # Find all manifest.yaml files
  find "$search_dir" -name "manifest.yaml" -type f 2>/dev/null | sort

  return 0
}

# Get manifest summary (one line per manifest)
# Arguments:
#   $1 - Manifest file path
# Outputs:
#   One-line summary: session_id | status | branch | last_activity
# Returns:
#   0 on success
get_manifest_summary() {
  local manifest="$1"

  validate_file "$manifest" "Manifest"

  local session_id status branch last_activity

  session_id=$(read_manifest_field "$manifest" "session_id")
  status=$(read_manifest_field "$manifest" "status")
  branch=$(read_manifest_nested "$manifest" "worktree" "branch")
  last_activity=$(read_manifest_field "$manifest" "last_activity")

  # Format as CSV-style
  echo "${session_id}|${status}|${branch}|${last_activity}"

  return 0
}

#######################################
# Claude Integration Functions
#######################################

# read_claude_session_id(manifest_path)
# Extract Claude session UUID from manifest
# Returns: UUID or empty string
read_claude_session_id() {
    local manifest="$1"

    if [[ ! -f "$manifest" ]]; then
        return 0
    fi

    # Extract claude.session_id using awk
    awk '
        /^claude:/ { in_claude=1; next }
        in_claude && /^  session_id:/ { print $2; exit }
        /^[a-z]/ && !/^claude:/ { in_claude=0 }
    ' "$manifest"
}

# update_claude_metadata(manifest_path, uuid, started_at, last_activity)
# Update or add Claude metadata section in manifest
update_claude_metadata() {
    local manifest="$1"
    local uuid="$2"
    local started_at="$3"
    local last_activity="$4"

    if [[ ! -f "$manifest" ]]; then
        log_error "Manifest not found: $manifest"
        return 1
    fi

    if [[ ! -w "$manifest" ]]; then
        log_error "Manifest not writable: $manifest"
        return 1
    fi

    local session_env="$HOME/.claude/session-env/$uuid"
    local file_history="$HOME/.claude/file-history/$uuid"

    # Check if claude section exists
    if grep -q "^claude:" "$manifest"; then
        # Update existing section
        sed -i "s|^  session_id:.*|  session_id: $uuid|" "$manifest"
        sed -i "s|^  last_activity:.*|  last_activity: $last_activity|" "$manifest"
    else
        # Add new section
        cat >> "$manifest" <<EOF

claude:
  session_id: $uuid
  session_env_path: $session_env
  file_history_path: $file_history
  started_at: $started_at
  last_activity: $last_activity
EOF
    fi
}

#######################################
# Tmux Integration Functions
#######################################

# read_tmux_session_name(manifest_path)
# Extract tmux session name from manifest
# Returns: Session name or empty string
read_tmux_session_name() {
    local manifest="$1"

    if [[ ! -f "$manifest" ]]; then
        return 0
    fi

    # Extract tmux.session_name using awk
    awk '
        /^tmux:/ { in_tmux=1; next }
        in_tmux && /^  session_name:/ { print $2; exit }
        /^[a-z]/ && !/^tmux:/ { in_tmux=0 }
    ' "$manifest"
}

# update_tmux_metadata(manifest_path, session_name, [window_name], [created_at])
# Update or add tmux metadata section in manifest
update_tmux_metadata() {
    local manifest="$1"
    local session_name="$2"
    local window_name="${3:-main}"
    local created_at="${4:-$(date -Iseconds)}"

    if [[ ! -f "$manifest" ]]; then
        log_error "Manifest not found: $manifest"
        return 1
    fi

    if [[ ! -w "$manifest" ]]; then
        log_error "Manifest not writable: $manifest"
        return 1
    fi

    # Check if tmux section exists
    if grep -q "^tmux:" "$manifest"; then
        # Update existing section
        sed -i "s|^  session_name:.*|  session_name: $session_name|" "$manifest"
    else
        # Add new section
        cat >> "$manifest" <<EOF

tmux:
  session_name: $session_name
  window_name: $window_name
  created_at: $created_at
EOF
    fi
}

# Export functions for use in other scripts
export -f read_manifest_field read_manifest_nested
export -f update_manifest_field update_manifest_nested_field
export -f update_manifest_activity add_manifest_artifact update_manifest_worktree_field
export -f create_manifest display_manifest list_manifests get_manifest_summary
export -f read_claude_session_id update_claude_metadata
export -f read_tmux_session_name update_tmux_metadata
