#!/usr/bin/env bash
#
# git-utils.sh - Git operations and worktree management utilities
#
# Provides wrapper functions for common git operations
#
# Usage:
#   source lib/git-utils.sh
#
# Dependencies:
#   lib/common-utils.sh

# Source guard to prevent double-sourcing
[[ -n "${GIT_UTILS_LOADED:-}" ]] && return 0
readonly GIT_UTILS_LOADED=1

set -euo pipefail

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common-utils.sh"

#######################################
# Git Repository Operations
#######################################

# Check if a directory is a git repository
# Arguments:
#   $1 - Directory path
# Returns:
#   0 if it's a git repo, 1 otherwise
is_git_repo() {
  local dir="$1"

  if [[ ! -d "$dir" ]]; then
    return 1
  fi

  cd "$dir" && git rev-parse --git-dir >/dev/null 2>&1
}

# Get the git repository root directory
# Arguments:
#   $1 - Directory path (optional, default: current directory)
# Outputs:
#   Repository root path
# Returns:
#   0 on success, 1 if not in a git repo
get_repo_root() {
  local dir="${1:-.}"

  validate_directory "$dir" "Directory"

  log_debug "Getting repo root for: $dir"

  local root
  root=$(cd "$dir" && git rev-parse --show-toplevel 2>/dev/null) || {
    log_error "Not in a git repository: $dir"
    return 1
  }

  echo "$root"
  return 0
}

# Get the current branch name
# Arguments:
#   $1 - Repository path (optional, default: current directory)
# Outputs:
#   Branch name
# Returns:
#   0 on success, 1 on error
get_current_branch() {
  local repo="${1:-.}"

  if ! is_git_repo "$repo"; then
    log_error "Not a git repository: $repo"
    return 1
  fi

  log_debug "Getting current branch in: $repo"

  local branch
  branch=$(cd "$repo" && git branch --show-current 2>/dev/null) || {
    log_error "Failed to get current branch"
    return 1
  }

  echo "$branch"
  return 0
}

# Get the current commit hash
# Arguments:
#   $1 - Repository path (optional, default: current directory)
#   $2 - Format: "short" or "long" (default: long)
# Outputs:
#   Commit hash
# Returns:
#   0 on success, 1 on error
get_current_commit() {
  local repo="${1:-.}"
  local format="${2:-long}"

  if ! is_git_repo "$repo"; then
    log_error "Not a git repository: $repo"
    return 1
  fi

  log_debug "Getting current commit in: $repo (format: $format)"

  local commit
  if [[ "$format" == "short" ]]; then
    commit=$(cd "$repo" && git rev-parse --short HEAD 2>/dev/null) || {
      log_error "Failed to get current commit"
      return 1
    }
  else
    commit=$(cd "$repo" && git rev-parse HEAD 2>/dev/null) || {
      log_error "Failed to get current commit"
      return 1
    }
  fi

  echo "$commit"
  return 0
}

#######################################
# Git Remote Operations
#######################################

# Get the remote URL for a repository
# Arguments:
#   $1 - Repository path (optional, default: current directory)
#   $2 - Remote name (optional, default: origin)
# Outputs:
#   Remote URL
# Returns:
#   0 on success, 1 on error
get_remote_url() {
  local repo="${1:-.}"
  local remote="${2:-origin}"

  if ! is_git_repo "$repo"; then
    log_error "Not a git repository: $repo"
    return 1
  fi

  log_debug "Getting remote URL for '$remote' in: $repo"

  local url
  url=$(cd "$repo" && git remote get-url "$remote" 2>/dev/null) || {
    log_error "Remote '$remote' not found"
    return 1
  }

  echo "$url"
  return 0
}

# Check if repository has a remote
# Arguments:
#   $1 - Repository path (optional, default: current directory)
#   $2 - Remote name (optional, default: origin)
# Returns:
#   0 if remote exists, 1 otherwise
has_remote() {
  local repo="${1:-.}"
  local remote="${2:-origin}"

  if ! is_git_repo "$repo"; then
    return 1
  fi

  cd "$repo" && git remote get-url "$remote" >/dev/null 2>&1
}

#######################################
# Git Worktree Operations
#######################################

# List all worktrees for a repository
# Arguments:
#   $1 - Repository path
# Outputs:
#   List of worktree paths (one per line)
# Returns:
#   0 on success
list_worktrees() {
  local repo="$1"

  if ! is_git_repo "$repo"; then
    log_error "Not a git repository: $repo"
    return 1
  fi

  log_debug "Listing worktrees for: $repo"

  cd "$repo" && git worktree list --porcelain 2>/dev/null | grep "^worktree " | cut -d' ' -f2-

  return 0
}

# Check if a worktree exists at a given path
# Arguments:
#   $1 - Repository path
#   $2 - Worktree path to check
# Returns:
#   0 if worktree exists, 1 otherwise
worktree_exists() {
  local repo="$1"
  local worktree_path="$2"

  if ! is_git_repo "$repo"; then
    return 1
  fi

  log_debug "Checking if worktree exists: $worktree_path"

  local worktrees
  worktrees=$(list_worktrees "$repo")

  echo "$worktrees" | grep -q "^${worktree_path}$"
}

# Add a new worktree
# Arguments:
#   $1 - Repository path
#   $2 - Worktree path
#   $3 - Branch name
#   $4 - Create new branch? (optional: "new" or "existing", default: existing)
# Returns:
#   0 on success, 1 on error
add_worktree() {
  local repo="$1"
  local worktree_path="$2"
  local branch="$3"
  local mode="${4:-existing}"

  validate_directory "$repo" "Repository"
  validate_not_empty "$worktree_path" "Worktree path"
  validate_not_empty "$branch" "Branch name"

  if ! is_git_repo "$repo"; then
    error_exit "Not a git repository: $repo"
  fi

  log_debug "Adding worktree: $worktree_path (branch: $branch, mode: $mode)"

  # Check if worktree already exists
  if [[ -d "$worktree_path" ]]; then
    log_error "Worktree path already exists: $worktree_path"
    return 1
  fi

  # Add worktree
  local git_cmd="cd \"$repo\" && git worktree add"

  if [[ "$mode" == "new" ]]; then
    git_cmd="$git_cmd -b \"$branch\" \"$worktree_path\""
  else
    git_cmd="$git_cmd \"$worktree_path\" \"$branch\""
  fi

  log_debug "Executing: $git_cmd"

  if eval "$git_cmd" 2>&1 | tee >(cat >&2); then
    log_success "Added worktree: $worktree_path"
    return 0
  else
    log_error "Failed to add worktree"
    return 1
  fi
}

# Remove a worktree
# Arguments:
#   $1 - Repository path
#   $2 - Worktree path
#   $3 - Force removal? (optional: "force" or empty, default: empty)
# Returns:
#   0 on success, 1 on error
remove_worktree() {
  local repo="$1"
  local worktree_path="$2"
  local force="${3:-}"

  validate_directory "$repo" "Repository"
  validate_not_empty "$worktree_path" "Worktree path"

  if ! is_git_repo "$repo"; then
    error_exit "Not a git repository: $repo"
  fi

  log_debug "Removing worktree: $worktree_path (force: $force)"

  # Build command
  local git_cmd="cd \"$repo\" && git worktree remove \"$worktree_path\""

  if [[ "$force" == "force" ]]; then
    git_cmd="$git_cmd --force"
  fi

  log_debug "Executing: $git_cmd"

  if eval "$git_cmd" 2>&1; then
    log_success "Removed worktree: $worktree_path"
    return 0
  else
    log_error "Failed to remove worktree"
    return 1
  fi
}

#######################################
# Git Status Operations
#######################################

# Check if repository has uncommitted changes
# Arguments:
#   $1 - Repository path (optional, default: current directory)
# Returns:
#   0 if there are changes, 1 if clean
has_uncommitted_changes() {
  local repo="${1:-.}"

  if ! is_git_repo "$repo"; then
    return 1
  fi

  log_debug "Checking for uncommitted changes in: $repo"

  ! cd "$repo" && git diff-index --quiet HEAD 2>/dev/null
}

# Check if repository has untracked files
# Arguments:
#   $1 - Repository path (optional, default: current directory)
# Returns:
#   0 if there are untracked files, 1 if none
has_untracked_files() {
  local repo="${1:-.}"

  if ! is_git_repo "$repo"; then
    return 1
  fi

  log_debug "Checking for untracked files in: $repo"

  local untracked
  untracked=$(cd "$repo" && git ls-files --others --exclude-standard 2>/dev/null)

  [[ -n "$untracked" ]]
}

# Get short status summary
# Arguments:
#   $1 - Repository path (optional, default: current directory)
# Outputs:
#   Status summary (e.g., "clean", "modified", "untracked files")
# Returns:
#   0 on success
get_status_summary() {
  local repo="${1:-.}"

  if ! is_git_repo "$repo"; then
    echo "not a git repository"
    return 1
  fi

  log_debug "Getting status summary for: $repo"

  local status_parts=()

  if has_uncommitted_changes "$repo"; then
    status_parts+=("uncommitted changes")
  fi

  if has_untracked_files "$repo"; then
    status_parts+=("untracked files")
  fi

  if [[ ${#status_parts[@]} -eq 0 ]]; then
    echo "clean"
  else
    local IFS=", "
    echo "${status_parts[*]}"
  fi

  return 0
}

# Export functions for use in other scripts
export -f is_git_repo get_repo_root get_current_branch get_current_commit
export -f get_remote_url has_remote
export -f list_worktrees worktree_exists add_worktree remove_worktree
export -f has_uncommitted_changes has_untracked_files get_status_summary
