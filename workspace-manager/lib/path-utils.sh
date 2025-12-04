#!/usr/bin/env bash
#
# path-utils.sh - Path manipulation and git URL parsing utilities
#
# Provides functions for parsing git URLs, building worktree paths,
# and handling path variable substitution.
#
# Usage:
#   source lib/path-utils.sh
#
# Dependencies:
#   lib/common-utils.sh

# Source guard to prevent double-sourcing
[[ -n "${PATH_UTILS_LOADED:-}" ]] && return 0
readonly PATH_UTILS_LOADED=1

set -euo pipefail

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common-utils.sh"

#######################################
# Git URL Parsing
#######################################

# Parse a git URL and extract components
# Arguments:
#   $1 - Git URL (HTTPS or SSH format)
# Outputs:
#   Multi-line output with format:
#     platform=github.com
#     user=username
#     repo=repository
# Returns:
#   0 on success, 1 on invalid URL
parse_git_url() {
  local url="$1"

  log_debug "Parsing git URL: $url"

  local platform user repo

  # HTTPS format: https://github.com/user/repo.git
  if [[ "$url" =~ ^https://([^/]+)/([^/]+)/([^/]+)(\.git)?$ ]]; then
    platform="${BASH_REMATCH[1]}"
    user="${BASH_REMATCH[2]}"
    repo="${BASH_REMATCH[3]}"

  # SSH format: git@github.com:user/repo.git
  elif [[ "$url" =~ ^git@([^:]+):([^/]+)/([^/]+)(\.git)?$ ]]; then
    platform="${BASH_REMATCH[1]}"
    user="${BASH_REMATCH[2]}"
    repo="${BASH_REMATCH[3]}"

  # Alternative SSH format: ssh://git@github.com/user/repo.git
  elif [[ "$url" =~ ^ssh://git@([^/]+)/([^/]+)/([^/]+)(\.git)?$ ]]; then
    platform="${BASH_REMATCH[1]}"
    user="${BASH_REMATCH[2]}"
    repo="${BASH_REMATCH[3]}"

  else
    log_error "Invalid git URL format: $url"
    log_error "Supported formats:"
    log_error "  - https://github.com/user/repo.git"
    log_error "  - git@github.com:user/repo.git"
    log_error "  - ssh://git@github.com/user/repo.git"
    return 1
  fi

  # Strip .git suffix if present
  repo="${repo%.git}"

  # Output parsed components
  echo "platform=$platform"
  echo "user=$user"
  echo "repo=$repo"

  log_debug "Parsed: platform=$platform, user=$user, repo=$repo"

  return 0
}

#######################################
# Repository Path Detection
#######################################

# Detect the relative path within a git repository
# Arguments:
#   $1 - Full path to check
#   $2 - Repository root path
# Outputs:
#   Relative path from repo root, or "." if at root
# Returns:
#   0 on success, 1 on error
get_repo_relative_path() {
  local full_path="$1"
  local repo_root="$2"

  log_debug "Getting relative path: full=$full_path, root=$repo_root"

  # Normalize paths (remove trailing slashes)
  full_path="${full_path%/}"
  repo_root="${repo_root%/}"

  # Check if full_path is under repo_root
  if [[ ! "$full_path" =~ ^"$repo_root"(/|$) ]]; then
    log_error "Path is not under repository root"
    log_error "  Path: $full_path"
    log_error "  Root: $repo_root"
    return 1
  fi

  # Calculate relative path
  local relative="${full_path#$repo_root}"
  relative="${relative#/}"  # Remove leading slash

  # If empty, we're at the root
  if [[ -z "$relative" ]]; then
    echo "."
  else
    echo "$relative"
  fi

  return 0
}

#######################################
# Worktree Path Building
#######################################

# Build a hierarchical worktree path
# Arguments:
#   $1 - Base worktrees directory (e.g., ~/worktrees)
#   $2 - Platform (e.g., github.com)
#   $3 - User (e.g., username)
#   $4 - Repository (e.g., myproject)
#   $5 - Branch name (e.g., feature-auth)
# Outputs:
#   Full worktree path
# Returns:
#   0 on success, exits on validation error
build_worktree_path() {
  local base_dir="$1"
  local platform="$2"
  local user="$3"
  local repo="$4"
  local branch="$5"

  log_debug "Building worktree path: base=$base_dir, platform=$platform, user=$user, repo=$repo, branch=$branch"

  # Validate inputs
  validate_not_empty "$base_dir" "Base directory"
  validate_not_empty "$platform" "Platform"
  validate_not_empty "$user" "User"
  validate_not_empty "$repo" "Repository"
  validate_not_empty "$branch" "Branch"

  # Sanitize branch name (replace / with -)
  local sanitized_branch="${branch//\//-}"

  log_debug "Sanitized branch: $branch -> $sanitized_branch"

  # Build path
  local worktree_path="$base_dir/$platform/$user/$repo/$sanitized_branch"

  # Validate path length
  validate_path_length "$worktree_path" "Worktree path"

  echo "$worktree_path"

  return 0
}

# Build a hierarchical source repository path
# Arguments:
#   $1 - Base src directory (e.g., ~/src)
#   $2 - Platform (e.g., github.com)
#   $3 - User (e.g., username)
#   $4 - Repository (e.g., myproject)
# Outputs:
#   Full source repository path
# Returns:
#   0 on success, exits on validation error
build_repo_path() {
  local base_dir="$1"
  local platform="$2"
  local user="$3"
  local repo="$4"

  log_debug "Building repo path: base=$base_dir, platform=$platform, user=$user, repo=$repo"

  # Validate inputs
  validate_not_empty "$base_dir" "Base directory"
  validate_not_empty "$platform" "Platform"
  validate_not_empty "$user" "User"
  validate_not_empty "$repo" "Repository"

  # Build path
  local repo_path="$base_dir/$platform/$user/$repo"

  # Validate path length
  validate_path_length "$repo_path" "Repository path"

  echo "$repo_path"

  return 0
}

#######################################
# Path Variable Substitution
#######################################

# Substitute environment variables in a path for portability
# Arguments:
#   $1 - Path containing environment variables (e.g., $HOME/project)
# Outputs:
#   Path with variables expanded
# Returns:
#   0 on success
substitute_path_vars() {
  local path="$1"

  log_debug "Substituting path variables: $path"

  # Use eval to expand variables (in a safe, read-only way)
  local expanded_path
  expanded_path=$(eval echo "$path")

  echo "$expanded_path"

  return 0
}

# Make a path portable by replacing home directory with $HOME
# Arguments:
#   $1 - Absolute path
# Outputs:
#   Portable path with $HOME variable
# Returns:
#   0 on success
make_path_portable() {
  local path="$1"

  log_debug "Making path portable: $path"

  # Replace home directory with $HOME
  local portable_path="${path/#$HOME/\$HOME}"

  echo "$portable_path"

  return 0
}

# Expand a portable path to absolute path
# Arguments:
#   $1 - Portable path (may contain $HOME, ~, or be absolute)
# Outputs:
#   Absolute path with variables expanded
# Returns:
#   0 on success
expand_path() {
  local path="$1"

  log_debug "Expanding path: $path"

  # Handle tilde expansion
  if [[ "$path" =~ ^\~ ]]; then
    path="${path/#\~/$HOME}"
  fi

  # Handle $HOME expansion
  if [[ "$path" =~ \$HOME ]]; then
    path="${path//\$HOME/$HOME}"
  fi

  # Convert to absolute path (resolve . and ..)
  if [[ -e "$path" ]]; then
    path="$(cd "$path" && pwd)"
  elif [[ -e "$(dirname "$path")" ]]; then
    # Parent exists, combine parent's absolute path with basename
    local parent_abs
    parent_abs="$(cd "$(dirname "$path")" && pwd)"
    local basename
    basename="$(basename "$path")"
    path="$parent_abs/$basename"
  fi

  echo "$path"

  return 0
}

#######################################
# Branch Name Utilities
#######################################

# Sanitize a branch name for use in filesystem paths
# Arguments:
#   $1 - Branch name (may contain / and other special chars)
# Outputs:
#   Sanitized branch name safe for filesystem
# Returns:
#   0 on success
sanitize_branch_name() {
  local branch="$1"

  log_debug "Sanitizing branch name: $branch"

  # Replace / with -
  local sanitized="${branch//\//-}"

  # Replace other problematic characters with -
  sanitized="${sanitized//[: ]/-}"

  # Remove leading/trailing dashes
  sanitized="${sanitized#-}"
  sanitized="${sanitized%-}"

  # Collapse multiple consecutive dashes
  sanitized="$(echo "$sanitized" | sed 's/-\+/-/g')"

  log_debug "Sanitized: $branch -> $sanitized"

  echo "$sanitized"

  return 0
}

# Export functions for use in other scripts
export -f parse_git_url get_repo_relative_path
export -f build_worktree_path build_repo_path
export -f substitute_path_vars make_path_portable expand_path
export -f sanitize_branch_name
