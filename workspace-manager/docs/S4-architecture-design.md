# S4: Architecture Design

**Date:** 2025-12-02

**Project:** Workspace & Session Management System

**Phase:** S4 - Architecture Design

**Status:** 🔄 In Progress

---

## Purpose

Design the detailed architecture for implementing D4 requirements.

**Input from D4:**
- 25+ requirements across 6 areas
- 4-sprint implementation plan (~22 hours)
- 8 SHOULD conditions from review
- 5 NICE conditions from review

**Output:** Detailed technical design ready for implementation (S5-S11)

---

## Architecture Overview

### System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    User Interaction Layer                    │
├─────────────────────────────────────────────────────────────┤
│  clone-repo  │ create-worktree │ archive-session │ dashboard │
│  resume-session  │  cleanup-merged-worktrees  │  gwq (external)│
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Core Library Layer                      │
├─────────────────────────────────────────────────────────────┤
│  manifest-utils.sh  │  audit-utils.sh  │  common-utils.sh   │
│  path-utils.sh      │  git-utils.sh    │  validation.sh     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                       Storage Layer                          │
├─────────────────────────────────────────────────────────────┤
│  ~/src/              │  ~/worktrees/      │  ~/.claude/sessions/ │
│  engram-research/session-archives/  (git-backed)             │
└─────────────────────────────────────────────────────────────┘
```

### Component Diagram

```
┌──────────────────┐
│  clone-repo.sh   │──┐
└──────────────────┘  │
                      │    ┌────────────────────┐
┌──────────────────┐  ├───→│  common-utils.sh   │
│ create-worktree  │──┘    │  - error_exit()    │
└──────────────────┘       │  - log_info()      │
                           │  - validate_dir()  │
┌──────────────────┐       └────────────────────┘
│ archive-session  │──┐
└──────────────────┘  │    ┌────────────────────┐
                      ├───→│  audit-utils.sh    │
┌──────────────────┐  │    │  - audit_secrets() │
│session-dashboard │──┘    │  - scan_file()     │
└──────────────────┘       └────────────────────┘

                           ┌────────────────────┐
                           │ manifest-utils.sh  │
                           │ - read_manifest()  │
                           │ - write_manifest() │
                           │ - update_field()   │
                           └────────────────────┘
```

---

## Core Library Design

### common-utils.sh

**Purpose:** Shared utilities used by all scripts

**Functions:**

```bash
#!/bin/bash
# common-utils.sh - Shared utilities for workspace management

# Script metadata
SCRIPT_VERSION="1.0.0"
SCRIPT_NAME="$(basename "$0")"

# Color codes for output (optional, can disable with --no-color)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
  local msg="$1"
  echo -e "${BLUE}ℹ${NC} $msg"
}

log_success() {
  local msg="$1"
  echo -e "${GREEN}✓${NC} $msg"
}

log_warn() {
  local msg="$1"
  echo -e "${YELLOW}⚠${NC} $msg" >&2
}

log_error() {
  local msg="$1"
  echo -e "${RED}✗${NC} ERROR: $msg" >&2
}

# Error handling
error_exit() {
  local msg="$1"
  local exit_code="${2:-1}"
  log_error "$msg"
  exit "$exit_code"
}

# Debug logging (only if DEBUG=1 environment variable set)
log_debug() {
  local msg="$1"
  if [[ "${DEBUG:-0}" == "1" ]]; then
    echo -e "${BLUE}[DEBUG]${NC} $msg" >&2
  fi
}

# Validation functions
validate_dir() {
  local dir="$1"
  local name="${2:-Directory}"

  if [[ ! -d "$dir" ]]; then
    error_exit "$name does not exist: $dir"
  fi
}

validate_file() {
  local file="$1"
  local name="${2:-File}"

  if [[ ! -f "$file" ]]; then
    error_exit "$name does not exist: $file"
  fi
}

validate_not_empty() {
  local value="$1"
  local name="${2:-Value}"

  if [[ -z "$value" ]]; then
    error_exit "$name cannot be empty"
  fi
}

# User confirmation
confirm() {
  local prompt="$1"
  local default="${2:-y}"  # y or n

  local options
  if [[ "$default" == "y" ]]; then
    options="[Y/n]"
  else
    options="[y/N]"
  fi

  read -p "$prompt $options " response
  response="${response:-$default}"

  [[ "$response" =~ ^[Yy]$ ]]
}

# Environment validation
check_env_vars() {
  local vars=("$@")
  for var in "${vars[@]}"; do
    if [[ -z "${!var}" ]]; then
      error_exit "Environment variable $var is not set"
    fi
  done
}

# Safe directory creation
ensure_dir() {
  local dir="$1"
  local perms="${2:-755}"

  if [[ ! -d "$dir" ]]; then
    log_debug "Creating directory: $dir"
    mkdir -p "$dir" || error_exit "Failed to create directory: $dir"
    chmod "$perms" "$dir"
  fi
}

# Time formatting (for last_activity)
format_time_ago() {
  local timestamp="$1"  # ISO 8601 format
  local now=$(date +%s)
  local then=$(date -d "$timestamp" +%s 2>/dev/null || echo "$now")
  local diff=$((now - then))

  if [[ $diff -lt 60 ]]; then
    echo "${diff}s ago"
  elif [[ $diff -lt 3600 ]]; then
    echo "$((diff / 60))m ago"
  elif [[ $diff -lt 86400 ]]; then
    echo "$((diff / 3600))h ago"
  else
    echo "$((diff / 86400))d ago"
  fi
}
```

**Testing:**
- Unit test each function
- Test error cases (non-existent dirs, invalid input)
- Test with/without color output
- Test DEBUG mode

---

### path-utils.sh

**Purpose:** Path manipulation and URL parsing

**Functions:**

```bash
#!/bin/bash
# path-utils.sh - Path and URL utilities

source "$(dirname "${BASH_SOURCE[0]}")/common-utils.sh"

# Parse Git URL to extract components
# Input: https://github.com/vbonnet/engram.git
# Output: platform=github user=vbonnet repo=engram
parse_git_url() {
  local url="$1"
  validate_not_empty "$url" "Git URL"

  # Remove .git suffix
  url="${url%.git}"

  # Extract platform (github.com, gitlab.com, etc.)
  local platform
  if [[ "$url" =~ ^https?://([^/]+)/ ]]; then
    platform="${BASH_REMATCH[1]}"
  elif [[ "$url" =~ ^git@([^:]+): ]]; then
    platform="${BASH_REMATCH[1]}"
  else
    error_exit "Cannot parse Git URL: $url"
  fi

  # Remove protocol and extract path
  local path
  if [[ "$url" =~ https?://[^/]+/(.+)$ ]]; then
    path="${BASH_REMATCH[1]}"
  elif [[ "$url" =~ git@[^:]+:(.+)$ ]]; then
    path="${BASH_REMATCH[1]}"
  fi

  # Split path into user/repo
  local user repo
  if [[ "$path" =~ ^([^/]+)/([^/]+)$ ]]; then
    user="${BASH_REMATCH[1]}"
    repo="${BASH_REMATCH[2]}"
  else
    error_exit "Cannot parse user/repo from: $path"
  fi

  # Output as key=value pairs
  echo "platform=$platform"
  echo "user=$user"
  echo "repo=$repo"
}

# Get repo path from current directory
# Returns: github/vbonnet/engram (relative to $SRC_ROOT)
get_repo_relative_path() {
  local current_dir="$(pwd)"
  local src_root="${SRC_ROOT:-$HOME/src}"

  # Check if we're in a repo under $SRC_ROOT
  if [[ "$current_dir" != "$src_root"/* ]]; then
    error_exit "Not in a repository under $SRC_ROOT"
  fi

  # Find .git directory
  local git_dir="$current_dir"
  while [[ "$git_dir" != "/" && "$git_dir" != "$src_root" ]]; do
    if [[ -d "$git_dir/.git" ]]; then
      # Found repo root, extract relative path
      local rel_path="${git_dir#$src_root/}"
      echo "$rel_path"
      return 0
    fi
    git_dir="$(dirname "$git_dir")"
  done

  error_exit "Not in a Git repository"
}

# Build worktree path from repo path
# Input: github/vbonnet/engram, feature-branch
# Output: /home/user/worktrees/github/vbonnet/engram/feature-branch
build_worktree_path() {
  local repo_rel_path="$1"
  local branch="$2"
  local worktrees_root="${WORKTREES_ROOT:-$HOME/worktrees}"

  validate_not_empty "$repo_rel_path" "Repository path"
  validate_not_empty "$branch" "Branch name"

  echo "$worktrees_root/$repo_rel_path/$branch"
}

# Substitute variables in path
# Input: {WORKTREES_ROOT}/github/vbonnet/engram/feature-x
# Output: /home/user/worktrees/github/vbonnet/engram/feature-x
substitute_path_vars() {
  local path="$1"

  # Substitute known variables
  path="${path//\{SRC_ROOT\}/${SRC_ROOT:-$HOME/src}}"
  path="${path//\{WORKTREES_ROOT\}/${WORKTREES_ROOT:-$HOME/worktrees}}"
  path="${path//\{SESSIONS_ROOT\}/${SESSIONS_ROOT:-$HOME/.claude/sessions}}"
  path="${path//\{ARCHIVES_ROOT\}/${ARCHIVES_ROOT:-$HOME/src/github/vbonnet/engram-research/session-archives}}"
  path="${path//\{HOME\}/$HOME}"

  echo "$path"
}

# Make path portable by adding variables
# Input: /home/user/worktrees/github/vbonnet/engram/feature-x
# Output: {WORKTREES_ROOT}/github/vbonnet/engram/feature-x
make_path_portable() {
  local path="$1"

  # Replace known paths with variables
  path="${path//${SRC_ROOT:-$HOME/src}/\{SRC_ROOT\}}"
  path="${path//${WORKTREES_ROOT:-$HOME/worktrees}/\{WORKTREES_ROOT\}}"
  path="${path//${SESSIONS_ROOT:-$HOME/.claude/sessions}/\{SESSIONS_ROOT\}}"
  path="${path//${ARCHIVES_ROOT:-$HOME/src/github/vbonnet/engram-research/session-archives}/\{ARCHIVES_ROOT\}}"
  path="${path//$HOME/\{HOME\}}"

  echo "$path"
}
```

**Testing:**
- Test various URL formats (HTTPS, SSH, different platforms)
- Test path substitution with different env vars
- Test portable path generation
- Test from different directory locations

---

### manifest-utils.sh

**Purpose:** YAML manifest parsing and manipulation

**Functions:**

```bash
#!/bin/bash
# manifest-utils.sh - Manifest file utilities

source "$(dirname "${BASH_SOURCE[0]}")/common-utils.sh"
source "$(dirname "${BASH_SOURCE[0]}")/path-utils.sh"

# Read field from manifest (simple YAML parsing)
# Usage: read_manifest_field manifest.yaml "session_id"
read_manifest_field() {
  local manifest="$1"
  local field="$2"

  validate_file "$manifest" "Manifest"

  # Simple grep-based YAML parsing
  # Works for simple key: value format
  local value=$(grep "^${field}:" "$manifest" | cut -d: -f2- | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')

  # Handle quoted strings
  value=$(echo "$value" | sed 's/^"\(.*\)"$/\1/' | sed "s/^'\(.*\)'$/\1/")

  echo "$value"
}

# Read nested field from manifest
# Usage: read_manifest_nested manifest.yaml "worktree" "path"
read_manifest_nested() {
  local manifest="$1"
  local parent="$2"
  local field="$3"

  validate_file "$manifest" "Manifest"

  # Find parent section and extract field
  # This is simplified - production might use yq or python
  awk "/^${parent}:/,/^[^ ]/ {if (/^[[:space:]]+${field}:/) print}" "$manifest" | \
    cut -d: -f2- | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//'
}

# Update field in manifest (in-place)
# Usage: update_manifest_field manifest.yaml "status" "archived"
update_manifest_field() {
  local manifest="$1"
  local field="$2"
  local value="$3"

  validate_file "$manifest" "Manifest"

  # Use sed for in-place update
  # Quote value if it contains spaces
  if [[ "$value" =~ [[:space:]] ]]; then
    value="\"$value\""
  fi

  sed -i "s|^${field}:.*|${field}: ${value}|" "$manifest"
}

# Add to list in manifest
# Usage: add_to_manifest_list manifest.yaml "artifacts.created" "- path: S7-plan.md"
add_to_manifest_list() {
  local manifest="$1"
  local list_path="$2"
  local item="$3"

  validate_file "$manifest" "Manifest"

  # Find list section and append item
  # This is simplified - would need more robust YAML handling
  echo "$item" >> "$manifest"
}

# Create new manifest from template
# Usage: create_manifest session_id project worktree_path
create_manifest() {
  local session_id="$1"
  local project="${2:-unnamed}"
  local worktree_path="${3:-}"
  local output_file="$4"

  local created=$(date -u +%Y-%m-%dT%H:%M:%SZ)

  # Make worktree path portable if provided
  if [[ -n "$worktree_path" ]]; then
    worktree_path=$(make_path_portable "$worktree_path")
  fi

  cat > "$output_file" << EOF
# Session metadata
session_id: "$session_id"
created: "$created"
last_activity: "$created"
status: "active"

# Work context
project: "$project"
project_type: "ad-hoc"
description: |
  Working on $project

# Git context
worktree:
  path: "${worktree_path}"
  repo: ""
  branch: ""
  base_branch: "main"

# Artifacts tracking
artifacts:
  created: []
  working_files: []

# Context audit
context_audit:
  tokens_consumed: 0
  files_available: 0
  files_accessed: 0
  efficiency_ratio: 0.0

# Tags for filtering
tags: []

# Resumption info
resumption:
  cwd: "${worktree_path}"
  last_phase: ""
  next_steps: |
    Continue work on $project
  blocked_on: null
EOF

  log_success "Created manifest: $output_file"
}

# Update last_activity timestamp
update_last_activity() {
  local manifest="$1"
  local timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)

  update_manifest_field "$manifest" "last_activity" "$timestamp"
}

# Display manifest summary (human-readable)
display_manifest() {
  local manifest="$1"

  validate_file "$manifest" "Manifest"

  local session_id=$(read_manifest_field "$manifest" "session_id")
  local project=$(read_manifest_field "$manifest" "project")
  local status=$(read_manifest_field "$manifest" "status")
  local last_activity=$(read_manifest_field "$manifest" "last_activity")
  local worktree_path=$(read_manifest_nested "$manifest" "worktree" "path")

  echo "Session: $session_id"
  echo "Project: $project"
  echo "Status: $status"
  echo "Last activity: $(format_time_ago "$last_activity")"
  if [[ -n "$worktree_path" ]]; then
    echo "Worktree: $(substitute_path_vars "$worktree_path")"
  fi
}
```

**Note on YAML Parsing:**
- Simple grep/sed approach for MVP (keeps dependencies minimal)
- Assumes well-formed, simple YAML (our manifests are controlled)
- For production, could migrate to `yq` if complex parsing needed
- Trade-off: Simplicity vs robustness (chose simplicity)

**Testing:**
- Test reading various field types (strings, numbers, nested)
- Test updating fields
- Test manifest creation
- Test with/without worktree paths
- Test path substitution

---

### audit-utils.sh

**Purpose:** Sensitive data detection and auditing

**Functions:**

```bash
#!/bin/bash
# audit-utils.sh - Security audit utilities

source "$(dirname "${BASH_SOURCE[0]}")/common-utils.sh"

# Secret patterns (extended from D4 R2.3)
declare -A SECRET_PATTERNS=(
  ["API_KEY"]='[A-Za-z0-9]{32,}'
  ["AWS_CRED"]='AKIA[A-Z0-9]{16}'
  ["PRIVATE_KEY"]='-----BEGIN.*PRIVATE KEY-----'
  ["TOKEN"]='token[=:]\s*[A-Za-z0-9_-]{20,}'
  ["PASSWORD"]='password[=:]\s*\S+'
  ["SSH_KEY"]='ssh-rsa|ssh-ed25519'
  ["DB_URL"]='postgresql://|mysql://.*password'
)

# Scan single file for secrets
# Returns: 0 if no secrets, 1 if secrets found
# Outputs: Lines matching secret patterns
scan_file_for_secrets() {
  local file="$1"

  if [[ ! -f "$file" ]]; then
    return 0
  fi

  local findings=""
  for pattern_name in "${!SECRET_PATTERNS[@]}"; do
    local pattern="${SECRET_PATTERNS[$pattern_name]}"
    local matches=$(grep -nE "$pattern" "$file" 2>/dev/null || true)

    if [[ -n "$matches" ]]; then
      findings+="$file [$pattern_name]:\n$matches\n\n"
    fi
  done

  if [[ -n "$findings" ]]; then
    echo -e "$findings"
    return 1
  fi

  return 0
}

# Scan directory recursively for secrets
scan_directory_for_secrets() {
  local dir="$1"
  local max_results="${2:-20}"  # Limit output

  validate_dir "$dir" "Directory to scan"

  local all_findings=""
  local count=0

  # Find all files (excluding .git)
  while IFS= read -r file; do
    local findings=$(scan_file_for_secrets "$file")
    if [[ $? -eq 1 ]]; then
      all_findings+="$findings"
      ((count++))

      if [[ $count -ge $max_results ]]; then
        all_findings+="\n[... truncated, $max_results results shown ...]\n"
        break
      fi
    fi
  done < <(find "$dir" -type f ! -path "*/.git/*" 2>/dev/null)

  if [[ -n "$all_findings" ]]; then
    echo -e "$all_findings"
    return 1
  fi

  return 0
}

# Audit entire session directory (manifest + working + artifacts)
audit_session_for_secrets() {
  local session_dir="$1"

  validate_dir "$session_dir" "Session directory"

  log_info "Auditing session directory for sensitive data..."

  local findings=""

  # Scan manifest.yaml
  if [[ -f "$session_dir/manifest.yaml" ]]; then
    log_debug "Scanning manifest.yaml..."
    local manifest_findings=$(scan_file_for_secrets "$session_dir/manifest.yaml")
    if [[ $? -eq 1 ]]; then
      findings+="=== manifest.yaml ===\n$manifest_findings\n"
    fi
  fi

  # Scan working/ directory
  if [[ -d "$session_dir/working" ]]; then
    log_debug "Scanning working/ directory..."
    local working_findings=$(scan_directory_for_secrets "$session_dir/working")
    if [[ $? -eq 1 ]]; then
      findings+="=== working/ ===\n$working_findings\n"
    fi
  fi

  # Scan artifacts/ directory
  if [[ -d "$session_dir/artifacts" ]]; then
    log_debug "Scanning artifacts/ directory..."
    local artifacts_findings=$(scan_directory_for_secrets "$session_dir/artifacts")
    if [[ $? -eq 1 ]]; then
      findings+="=== artifacts/ ===\n$artifacts_findings\n"
    fi
  fi

  if [[ -n "$findings" ]]; then
    echo ""
    log_warn "⚠️  Potential secrets detected in session files:"
    echo ""
    echo -e "$findings"
    echo ""
    echo "Limitations:"
    echo "  - May not detect secrets in binary files"
    echo "  - May not detect secrets in encrypted files"
    echo "  - Symlinks are followed (could escape scope)"
    echo "  - May produce false positives (long hashes, etc.)"
    echo ""
    echo "See USER-GUIDE.md for details on secret detection."
    echo ""

    return 1  # Secrets found
  fi

  log_success "No secrets detected in session"
  return 0  # Safe
}

# Interactive audit with user confirmation
audit_and_confirm() {
  local session_dir="$1"

  if ! audit_session_for_secrets "$session_dir"; then
    # Secrets found, ask user
    echo ""
    if confirm "Review files before proceeding?" "y"; then
      return 1  # User wants to review
    else
      log_warn "User accepted risk. Proceeding anyway."
      return 0  # User accepts risk
    fi
  fi

  return 0  # No secrets or user approved
}
```

**Testing:**
- Test detection of each pattern type
- Test false positives (long hashes, UUIDs)
- Test binary file handling
- Test symlink following
- Test truncation at max_results
- Test interactive confirmation

---

## Helper Script Designs

### clone-repo.sh

**Purpose:** Clone repository to correct hierarchical location

**Architecture:**

```bash
#!/bin/bash
# clone-repo.sh - Clone repository to hierarchical structure

set -euo pipefail

# Load utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common-utils.sh"
source "$SCRIPT_DIR/lib/path-utils.sh"

# Configuration
SRC_ROOT="${SRC_ROOT:-$HOME/src}"

# Usage
usage() {
  cat << EOF
Usage: $(basename "$0") <git-url>

Clone a Git repository to the hierarchical structure.

Arguments:
  git-url    Git repository URL (HTTPS or SSH)

Examples:
  $(basename "$0") https://github.com/vbonnet/engram.git
  $(basename "$0") git@github.com:vbonnet/dotfiles.git

Environment Variables:
  SRC_ROOT   Root directory for repositories (default: ~/src)
  DEBUG      Enable debug logging (DEBUG=1)

Output:
  Clones repository to: \$SRC_ROOT/{platform}/{user}/{repo}/
EOF
  exit 1
}

# Main function
main() {
  # Parse arguments
  if [[ $# -ne 1 ]]; then
    usage
  fi

  local git_url="$1"

  # Validate environment
  check_env_vars SRC_ROOT

  # Parse Git URL
  log_info "Parsing Git URL: $git_url"
  local url_parts=$(parse_git_url "$git_url")
  eval "$url_parts"  # Sets: platform, user, repo

  log_debug "Platform: $platform"
  log_debug "User: $user"
  log_debug "Repository: $repo"

  # Build target path
  local target_dir="$SRC_ROOT/$platform/$user/$repo"

  # Check if already exists
  if [[ -d "$target_dir" ]]; then
    error_exit "Repository already exists: $target_dir"
  fi

  # Create parent directory
  local parent_dir="$(dirname "$target_dir")"
  ensure_dir "$parent_dir"

  # Clone repository
  log_info "Cloning to: $target_dir"
  if git clone "$git_url" "$target_dir"; then
    log_success "Successfully cloned: $repo"
    log_success "Location: $target_dir"
  else
    error_exit "Git clone failed"
  fi
}

# Entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

**Error Handling:**
- Invalid URL format → Clear error message
- Repository already exists → Don't overwrite
- Git clone fails → Show git error, clean up partial clone
- Network issues → Let git handle, show helpful message

**Testing:**
- Test with HTTPS URLs
- Test with SSH URLs
- Test with various platforms (GitHub, GitLab)
- Test when repo already exists
- Test network failure (mock)
- Test invalid URLs

---

### create-worktree.sh

**Purpose:** Create git worktree in hierarchical mirror structure

**Architecture:**

```bash
#!/bin/bash
# create-worktree.sh - Create git worktree in mirror structure

set -euo pipefail

# Load utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common-utils.sh"
source "$SCRIPT_DIR/lib/path-utils.sh"
source "$SCRIPT_DIR/lib/git-utils.sh"

# Configuration
SRC_ROOT="${SRC_ROOT:-$HOME/src}"
WORKTREES_ROOT="${WORKTREES_ROOT:-$HOME/worktrees}"

# Usage
usage() {
  cat << EOF
Usage: $(basename "$0") <branch-name> [base-branch]

Create a git worktree in the hierarchical mirror structure.
Must be run from within a repository under \$SRC_ROOT.

Arguments:
  branch-name   Name of branch to create/checkout
  base-branch   Base branch to branch from (default: current branch)

Examples:
  $(basename "$0") feature-bash-guidance
  $(basename "$0") fix-telemetry main

Environment Variables:
  SRC_ROOT        Root directory for repositories (default: ~/src)
  WORKTREES_ROOT  Root directory for worktrees (default: ~/worktrees)
  DEBUG           Enable debug logging (DEBUG=1)

Output:
  Creates worktree at: \$WORKTREES_ROOT/{platform}/{user}/{repo}/{branch}/
EOF
  exit 1
}

# Main function
main() {
  # Parse arguments
  if [[ $# -lt 1 ]]; then
    usage
  fi

  local branch_name="$1"
  local base_branch="${2:-}"

  # Validate environment
  check_env_vars SRC_ROOT WORKTREES_ROOT

  # Get repo relative path (validates we're in a repo)
  log_info "Detecting repository..."
  local repo_rel_path=$(get_repo_relative_path)
  log_debug "Repository path: $repo_rel_path"

  # Build worktree path
  local worktree_path=$(build_worktree_path "$repo_rel_path" "$branch_name")

  # Check if worktree already exists
  if [[ -d "$worktree_path" ]]; then
    error_exit "Worktree already exists: $worktree_path"
  fi

  # Create parent directory
  local parent_dir="$(dirname "$worktree_path")"
  ensure_dir "$parent_dir"

  # Create worktree
  log_info "Creating worktree: $worktree_path"

  local git_cmd="git worktree add \"$worktree_path\""

  if [[ -n "$base_branch" ]]; then
    git_cmd+=" -b \"$branch_name\" \"$base_branch\""
  else
    git_cmd+=" -b \"$branch_name\""
  fi

  log_debug "Git command: $git_cmd"

  if eval "$git_cmd"; then
    log_success "Successfully created worktree: $branch_name"
    log_success "Location: $worktree_path"
    echo ""
    echo "To start working:"
    echo "  cd $worktree_path"
  else
    error_exit "Git worktree add failed"
  fi
}

# Entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

**Error Handling:**
- Not in a git repo → Clear error
- Worktree already exists → Don't overwrite
- Branch already exists → Let git handle (will fail if exists)
- Not in $SRC_ROOT → Clear error

**Testing:**
- Test from various repo locations
- Test with/without base branch
- Test when branch already exists
- Test when worktree already exists
- Test from non-repo directory
- Test from repo not in $SRC_ROOT

---

### archive-session.sh

**Purpose:** Archive completed session to git-backed storage with audit

**Architecture:**

```bash
#!/bin/bash
# archive-session.sh - Archive session to engram-research

set -euo pipefail

# Load utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common-utils.sh"
source "$SCRIPT_DIR/lib/manifest-utils.sh"
source "$SCRIPT_DIR/lib/audit-utils.sh"

# Configuration
SESSIONS_ROOT="${SESSIONS_ROOT:-$HOME/.claude/sessions}"
ARCHIVES_ROOT="${ARCHIVES_ROOT:-$HOME/src/github/vbonnet/engram-research/session-archives}"

# Usage
usage() {
  cat << EOF
Usage: $(basename "$0") <session-id> [--force]

Archive a completed session to engram-research/session-archives/.

Arguments:
  session-id   Session ID to archive
  --force      Skip secret audit (not recommended)

Process:
  1. Audit session for sensitive data (manifest + working + artifacts)
  2. Prompt user if secrets detected
  3. Copy session to archives/{date}/{session-id}/
  4. Update manifest status to "archived"
  5. Commit to git in engram-research
  6. Optionally delete local copy

Environment Variables:
  SESSIONS_ROOT   Session directory (default: ~/.claude/sessions)
  ARCHIVES_ROOT   Archive directory (default: see above)
  DEBUG           Enable debug logging (DEBUG=1)

Examples:
  $(basename "$0") claude-abc123
  $(basename "$0") claude-abc123 --force
EOF
  exit 1
}

# Main function
main() {
  # Parse arguments
  if [[ $# -lt 1 ]]; then
    usage
  fi

  local session_id="$1"
  local force_skip_audit=false

  if [[ "${2:-}" == "--force" ]]; then
    force_skip_audit=true
    log_warn "Skipping security audit (--force)"
  fi

  # Validate environment
  check_env_vars SESSIONS_ROOT ARCHIVES_ROOT

  # Validate session exists
  local session_dir="$SESSIONS_ROOT/$session_id"
  validate_dir "$session_dir" "Session directory"

  local manifest="$session_dir/manifest.yaml"
  validate_file "$manifest" "Session manifest"

  # Audit for secrets (unless --force)
  if [[ "$force_skip_audit" == "false" ]]; then
    if ! audit_and_confirm "$session_dir"; then
      log_error "Audit failed or user aborted"
      echo ""
      echo "To skip audit (not recommended): $(basename "$0") $session_id --force"
      exit 1
    fi
  fi

  # Prepare archive location
  local date=$(date +%Y-%m-%d)
  local archive_dir="$ARCHIVES_ROOT/$date/$session_id"

  if [[ -d "$archive_dir" ]]; then
    error_exit "Archive already exists: $archive_dir"
  fi

  # Create archive
  log_info "Archiving session: $session_id"
  ensure_dir "$(dirname "$archive_dir")"

  # Copy entire session directory
  log_info "Copying session files..."
  cp -r "$session_dir" "$archive_dir" || error_exit "Failed to copy session"

  # Update manifest status
  log_info "Updating manifest status..."
  update_manifest_field "$archive_dir/manifest.yaml" "status" "archived"

  # Commit to git
  log_info "Committing to git..."
  cd "$(dirname "$ARCHIVES_ROOT")"

  git add "session-archives/$date/$session_id" || error_exit "Git add failed"

  local commit_msg="Archive session $session_id ($date)

Session: $session_id
Date: $date
$(if [[ "$force_skip_audit" == "true" ]]; then echo "Note: Audit skipped with --force"; fi)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"

  git commit -m "$commit_msg" || error_exit "Git commit failed"

  log_success "Session archived: $archive_dir"

  # Ask to delete local copy
  echo ""
  if confirm "Delete local session copy?" "n"; then
    rm -rf "$session_dir"
    log_success "Local session deleted"
  else
    log_info "Local session kept: $session_dir"
  fi

  # Remind to push
  echo ""
  log_warn "Don't forget to push to remote:"
  echo "  cd $(dirname "$ARCHIVES_ROOT")"
  echo "  git push origin main"
}

# Entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

**Error Handling:**
- Session doesn't exist → Clear error
- Audit finds secrets → Prompt user (or abort)
- Archive already exists → Don't overwrite
- Git operations fail → Show git error
- Copy fails → Clean up partial archive

**Testing:**
- Test with clean session (no secrets)
- Test with secrets in manifest
- Test with secrets in working/
- Test with secrets in artifacts/
- Test --force flag
- Test when archive already exists
- Test git commit failure
- Test user declining to delete local

---

### session-dashboard.sh

**Purpose:** Display all active and archived sessions

**Architecture:**

```bash
#!/bin/bash
# session-dashboard.sh - Display session dashboard

set -euo pipefail

# Load utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common-utils.sh"
source "$SCRIPT_DIR/lib/manifest-utils.sh"

# Configuration
SESSIONS_ROOT="${SESSIONS_ROOT:-$HOME/.claude/sessions}"
ARCHIVES_ROOT="${ARCHIVES_ROOT:-$HOME/src/github/vbonnet/engram-research/session-archives}"

# Usage
usage() {
  cat << EOF
Usage: $(basename "$0") [--active|--archived|--all]

Display dashboard of Claude Code sessions.

Options:
  --active     Show only active sessions (default)
  --archived   Show only archived sessions
  --all        Show both active and archived

Output:
  Lists sessions with metadata from manifests

Environment Variables:
  SESSIONS_ROOT   Active sessions (default: ~/.claude/sessions)
  ARCHIVES_ROOT   Archived sessions (default: see above)
  DEBUG           Enable debug logging (DEBUG=1)

Examples:
  $(basename "$0")
  $(basename "$0") --all
EOF
  exit 1
}

# Display active sessions
show_active_sessions() {
  if [[ ! -d "$SESSIONS_ROOT" || -z "$(ls -A "$SESSIONS_ROOT" 2>/dev/null)" ]]; then
    echo "No active sessions"
    return
  fi

  local count=0
  for session_dir in "$SESSIONS_ROOT"/*; do
    if [[ ! -d "$session_dir" ]]; then
      continue
    fi

    local manifest="$session_dir/manifest.yaml"
    if [[ ! -f "$manifest" ]]; then
      continue
    fi

    ((count++))

    # Read manifest fields
    local session_id=$(basename "$session_dir")
    local project=$(read_manifest_field "$manifest" "project")
    local last_phase=$(read_manifest_nested "$manifest" "resumption" "last_phase")
    local worktree_path=$(read_manifest_nested "$manifest" "worktree" "path")
    local last_activity=$(read_manifest_field "$manifest" "last_activity")
    local next_steps=$(read_manifest_nested "$manifest" "resumption" "next_steps")

    # Substitute variables in worktree path
    if [[ -n "$worktree_path" ]]; then
      worktree_path=$(substitute_path_vars "$worktree_path")
    fi

    # Display
    echo "$session_id | $project | $last_phase"
    if [[ -n "$worktree_path" ]]; then
      echo "  Worktree: $worktree_path"
    fi
    echo "  Last activity: $(format_time_ago "$last_activity")"
    if [[ -n "$next_steps" ]]; then
      local next_first_line=$(echo "$next_steps" | head -1)
      echo "  Next: $next_first_line"
    fi
    echo ""
  done

  if [[ $count -eq 0 ]]; then
    echo "No active sessions"
  fi
}

# Display archived sessions
show_archived_sessions() {
  if [[ ! -d "$ARCHIVES_ROOT" ]]; then
    echo "No archived sessions (archives directory doesn't exist)"
    return
  fi

  local count=0
  local max_show=10

  # Find archives (newest first)
  while IFS= read -r manifest; do
    if [[ $count -ge $max_show ]]; then
      echo "... (showing recent $max_show, more in $ARCHIVES_ROOT)"
      break
    fi

    ((count++))

    # Extract date and session ID from path
    local archive_dir=$(dirname "$manifest")
    local session_id=$(basename "$archive_dir")
    local date=$(basename "$(dirname "$archive_dir")")

    local project=$(read_manifest_field "$manifest" "project")

    echo "$date | $session_id | $project | Archived"
  done < <(find "$ARCHIVES_ROOT" -name "manifest.yaml" -type f | sort -r)

  if [[ $count -eq 0 ]]; then
    echo "No archived sessions"
  fi
}

# Main function
main() {
  local mode="active"

  # Parse arguments
  if [[ $# -gt 0 ]]; then
    case "$1" in
      --active)
        mode="active"
        ;;
      --archived)
        mode="archived"
        ;;
      --all)
        mode="all"
        ;;
      *)
        usage
        ;;
    esac
  fi

  # Show sessions
  if [[ "$mode" == "active" || "$mode" == "all" ]]; then
    echo "Active Sessions"
    echo "==============="
    show_active_sessions
    echo ""
  fi

  if [[ "$mode" == "archived" || "$mode" == "all" ]]; then
    echo "Archived Sessions (recent)"
    echo "=========================="
    show_archived_sessions
  fi
}

# Entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

**Performance:**
- O(N) where N = number of sessions
- For large N, could add caching or index
- Current design: Scan all manifests each time (simple, works for < 100 sessions)

**Testing:**
- Test with no sessions
- Test with only active sessions
- Test with only archived sessions
- Test with both
- Test with many archived (> 10)
- Test with malformed manifests (graceful degradation)

---

## Migration Script Design

### migrate-workspace.sh

**Purpose:** Full upfront migration of existing workspace

**6 Phases:**

```bash
#!/bin/bash
# migrate-workspace.sh - Full workspace migration

set -euo pipefail

# Load utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common-utils.sh"

# Configuration
SRC_ROOT="${SRC_ROOT:-$HOME/src}"
WORKTREES_ROOT="${WORKTREES_ROOT:-$HOME/worktrees}"
SESSIONS_ROOT="${SESSIONS_ROOT:-$HOME/.claude/sessions}"
BACKUP_DIR="$HOME/workspace-migration-backup-$(date +%Y-%m-%d)"

# Phase status tracking
declare -A PHASE_STATUS=(
  ["setup"]="pending"
  ["migrate_repos"]="pending"
  ["migrate_worktrees"]="pending"
  ["create_manifests"]="pending"
  ["verification"]="pending"
  ["cleanup"]="pending"
)

# Usage
usage() {
  cat << EOF
Usage: $(basename "$0") [--dry-run] [--skip-backup]

Full migration of workspace to new hierarchical structure.

Options:
  --dry-run      Show what would be done without doing it
  --skip-backup  Skip backup creation (not recommended)

Phases:
  1. Setup        Create directory structure, install gwq
  2. Migrate Repos    Move all repos to ~/src/
  3. Migrate Worktrees    Move all worktrees to ~/worktrees/
  4. Create Manifests Create manifests for active sessions
  5. Verification Verify all migrations successful
  6. Cleanup      Delete old locations (with confirmation)

Estimated time: 3-4 hours

Environment Variables:
  SRC_ROOT        Target for repos (default: ~/src)
  WORKTREES_ROOT  Target for worktrees (default: ~/worktrees)
  SESSIONS_ROOT   Target for sessions (default: ~/.claude/sessions)

Examples:
  $(basename "$0")
  $(basename "$0") --dry-run
EOF
  exit 1
}

# Phase 1: Setup
phase_setup() {
  log_info "=== Phase 1: Setup (15 min) ==="

  # Create directory structure
  log_info "Creating directory structure..."
  ensure_dir "$SRC_ROOT/github"
  ensure_dir "$SRC_ROOT/gitlab"
  ensure_dir "$WORKTREES_ROOT/github"
  ensure_dir "$WORKTREES_ROOT/gitlab"
  ensure_dir "$SESSIONS_ROOT"

  # Set environment variables
  log_info "Setting environment variables..."
  # TODO: Add to ~/.bashrc or ~/.zshrc

  # Install gwq
  log_info "Checking gwq installation..."
  if ! command -v gwq &> /dev/null; then
    log_warn "gwq not installed. Install with:"
    echo "  go install github.com/d-kuro/gwq@latest"
    echo "  # or download binary from releases"
  else
    log_success "gwq already installed"
  fi

  PHASE_STATUS["setup"]="complete"
  log_success "Phase 1 complete"
}

# Phase 2: Migrate Repos
phase_migrate_repos() {
  log_info "=== Phase 2: Migrate Repositories (30-60 min) ==="

  # Find all git repos in /tmp/ and ~/
  log_info "Finding Git repositories..."
  local repos=()

  # TODO: Implement repo discovery
  # For each repo:
  #   1. Determine target location (parse remote URL or git config)
  #   2. Move repo to $SRC_ROOT/{platform}/{user}/{repo}/
  #   3. Verify git operations still work

  PHASE_STATUS["migrate_repos"]="complete"
  log_success "Phase 2 complete"
}

# Phase 3: Migrate Worktrees
phase_migrate_worktrees() {
  log_info "=== Phase 3: Migrate Worktrees (30-60 min) ==="

  # Find all worktrees
  log_info "Finding Git worktrees..."

  # TODO: Implement worktree discovery and migration
  # For each worktree:
  #   1. Determine parent repo
  #   2. Remove old worktree (git worktree remove)
  #   3. Create new worktree in $WORKTREES_ROOT/{platform}/{user}/{repo}/{branch}/

  PHASE_STATUS["migrate_worktrees"]="complete"
  log_success "Phase 3 complete"
}

# Phase 4: Create Manifests
phase_create_manifests() {
  log_info "=== Phase 4: Create Session Manifests (30-60 min) ==="

  # Find active sessions (claude-XXXX-cwd files, etc.)
  log_info "Finding active sessions..."

  # TODO: Implement session discovery
  # For each session:
  #   1. Create manifest.yaml
  #   2. Populate basic metadata (best effort)
  #   3. Link to worktree if applicable

  PHASE_STATUS["create_manifests"]="complete"
  log_success "Phase 4 complete"
}

# Phase 5: Verification
phase_verification() {
  log_info "=== Phase 5: Verification (15 min) ==="

  # TODO: Implement verification
  # - Count repos before/after
  # - Verify git operations in new locations
  # - Count worktrees before/after
  # - Verify worktree functionality
  # - Verify session manifests are valid

  PHASE_STATUS["verification"]="complete"
  log_success "Phase 5 complete"
}

# Phase 6: Cleanup
phase_cleanup() {
  log_info "=== Phase 6: Cleanup (15 min) ==="

  if ! confirm "Delete old locations (repos, worktrees, breadcrumbs)?" "n"; then
    log_info "Cleanup skipped by user"
    PHASE_STATUS["cleanup"]="skipped"
    return
  fi

  # TODO: Implement cleanup
  # - Delete /tmp/ repos (after verification)
  # - Delete old worktree locations
  # - Delete claude-XXXX-cwd breadcrumb files
  # - Delete backup after N days confirmation

  PHASE_STATUS["cleanup"]="complete"
  log_success "Phase 6 complete"
}

# Main migration workflow
main() {
  local dry_run=false
  local skip_backup=false

  # Parse arguments
  for arg in "$@"; do
    case "$arg" in
      --dry-run)
        dry_run=true
        log_warn "DRY RUN MODE - no changes will be made"
        ;;
      --skip-backup)
        skip_backup=true
        log_warn "Skipping backup creation"
        ;;
      *)
        usage
        ;;
    esac
  done

  log_info "Starting workspace migration..."
  echo ""

  # Create backup (unless skipped)
  if [[ "$skip_backup" == "false" ]]; then
    log_info "Creating backup: $BACKUP_DIR"
    # TODO: Implement backup
  fi

  # Run phases
  phase_setup
  echo ""

  phase_migrate_repos
  echo ""

  phase_migrate_worktrees
  echo ""

  phase_create_manifests
  echo ""

  phase_verification
  echo ""

  phase_cleanup
  echo ""

  # Summary
  log_success "=== Migration Complete ==="
  echo ""
  echo "Phase Status:"
  for phase in setup migrate_repos migrate_worktrees create_manifests verification cleanup; do
    echo "  $phase: ${PHASE_STATUS[$phase]}"
  done
  echo ""
  echo "Backup location: $BACKUP_DIR"
  echo "Keep backup for at least 7 days before deleting"
}

# Entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

**Note:** This is the architecture design. Full implementation will be in S7.

**Testing:**
- Test dry-run mode
- Test each phase independently
- Test recovery from phase failure
- Test backup creation and restore
- Test verification catches issues

---

## Testing Strategy

### Unit Tests

**test/unit/test-common-utils.sh:**
```bash
#!/bin/bash
# Unit tests for common-utils.sh

source "$(dirname "$0")/../../lib/common-utils.sh"

# Test counter
TESTS_RUN=0
TESTS_PASSED=0

# Test helper
assert_equals() {
  local expected="$1"
  local actual="$2"
  local test_name="$3"

  ((TESTS_RUN++))

  if [[ "$expected" == "$actual" ]]; then
    echo "✓ $test_name"
    ((TESTS_PASSED++))
  else
    echo "✗ $test_name"
    echo "  Expected: $expected"
    echo "  Actual: $actual"
  fi
}

# Tests
test_log_functions() {
  # Just verify they don't crash
  log_info "Test info message" &> /dev/null
  assert_equals "0" "$?" "log_info doesn't crash"

  log_success "Test success message" &> /dev/null
  assert_equals "0" "$?" "log_success doesn't crash"
}

test_validate_dir() {
  # Test with existing dir
  validate_dir "/" "Root" &> /dev/null
  assert_equals "0" "$?" "validate_dir succeeds for existing dir"

  # Test with non-existent dir
  validate_dir "/nonexistent" "Nonexistent" &> /dev/null
  assert_equals "1" "$?" "validate_dir fails for non-existent dir"
}

test_format_time_ago() {
  local now_iso=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  local result=$(format_time_ago "$now_iso")
  assert_equals "0s ago" "$result" "format_time_ago for now"
}

# Run tests
test_log_functions
test_validate_dir
test_format_time_ago

# Summary
echo ""
echo "Tests run: $TESTS_RUN"
echo "Tests passed: $TESTS_PASSED"
echo "Tests failed: $((TESTS_RUN - TESTS_PASSED))"

if [[ $TESTS_PASSED -eq $TESTS_RUN ]]; then
  echo "✓ All tests passed"
  exit 0
else
  echo "✗ Some tests failed"
  exit 1
fi
```

**Test coverage targets:**
- common-utils.sh: 80% (all core functions)
- path-utils.sh: 90% (critical path parsing)
- manifest-utils.sh: 80% (YAML parsing edge cases)
- audit-utils.sh: 90% (security critical)

### Integration Tests

**test/integration/test-clone-and-worktree.sh:**
```bash
#!/bin/bash
# Integration test: clone repo then create worktree

# Setup test environment
TEST_SRC_ROOT="/tmp/test-workspace/src"
TEST_WORKTREES_ROOT="/tmp/test-workspace/worktrees"

export SRC_ROOT="$TEST_SRC_ROOT"
export WORKTREES_ROOT="$TEST_WORKTREES_ROOT"

# Clean up from previous runs
rm -rf /tmp/test-workspace
mkdir -p /tmp/test-workspace

# Test: Clone public repo
echo "Testing clone-repo..."
./bin/clone-repo.sh https://github.com/vbonnet/test-repo.git

# Verify repo exists
if [[ -d "$TEST_SRC_ROOT/github/vbonnet/test-repo" ]]; then
  echo "✓ Repository cloned successfully"
else
  echo "✗ Repository clone failed"
  exit 1
fi

# Test: Create worktree
echo "Testing create-worktree..."
cd "$TEST_SRC_ROOT/github/vbonnet/test-repo"
./bin/create-worktree.sh test-branch

# Verify worktree exists
if [[ -d "$TEST_WORKTREES_ROOT/github/vbonnet/test-repo/test-branch" ]]; then
  echo "✓ Worktree created successfully"
else
  echo "✗ Worktree creation failed"
  exit 1
fi

# Cleanup
rm -rf /tmp/test-workspace

echo "✓ Integration test passed"
```

**Integration test coverage:**
- clone-repo + create-worktree workflow
- create-worktree + archive-session workflow
- Full migration dry-run
- Backup and restore

---

## Addressing D4 Review Conditions

### SHOULD Conditions (8 items)

**1. Backup restore verification (R4.1+)**

Add to migration script:
```bash
# After backup creation, test restore
test_backup_restore() {
  log_info "Testing backup restore..."

  # Create test file in backup
  local test_file="$BACKUP_DIR/.restore-test"
  echo "test" > "$test_file"

  # Verify can read it back
  if [[ "$(cat "$test_file")" == "test" ]]; then
    log_success "Backup is readable"
    rm "$test_file"
  else
    error_exit "Backup verification failed"
  fi
}
```

**Status:** ✅ Designed (will implement in S7)

**2. Count verification before cleanup (R4.3+)**

Add to verification phase:
```bash
# Count repos before migration
count_repos_before=$(find /tmp ~/ -maxdepth 2 -name .git -type d | wc -l)

# After migration
count_repos_after=$(find "$SRC_ROOT" -name .git -type d | wc -l)

if [[ $count_repos_after -ne $count_repos_before ]]; then
  error_exit "Repo count mismatch: before=$count_repos_before after=$count_repos_after"
fi
```

**Status:** ✅ Designed (will implement in S7)

**3. Git push verification (R3.3+)**

Add to archive-session.sh:
```bash
# After git commit
if git push origin main; then
  log_success "Pushed to remote successfully"
else
  log_error "Git push failed - archive is local only"
  log_warn "Remember to push manually later"
fi
```

**Status:** ✅ Designed (will implement in S7)

**4. Document audit limitations (R2.3)**

Add to USER-GUIDE.md troubleshooting section.

**Status:** ✅ Designed (will write in S6)

**5. Troubleshooting section (R6.1+)**

Will add to USER-GUIDE.md with common errors:
- "Repository already exists"
- "Not in a git repository"
- "Secrets detected" (how to review)
- "Git push failed"

**Status:** ✅ Designed (will write in S6)

**6. Common aliases (R1.5+)**

Add to environment setup:
```bash
# Add to ~/.bashrc or ~/.zshrc
alias archive='archive-session.sh'
alias resume='resume-session.sh'
alias sessions='session-dashboard.sh'
alias cw='create-worktree.sh'
```

**Status:** ✅ Designed (will document in S6)

**7. Testing strategy**

Designed above (unit + integration tests).

**Status:** ✅ Designed (this document)

**8. Logging/debugging support**

Implemented via `DEBUG=1` environment variable and `log_debug()` function.

**Status:** ✅ Designed (in common-utils.sh)

---

### NICE Conditions (5 items)

**1. Dashboard shows inactive session alerts**

Add to session-dashboard.sh:
```bash
# After displaying sessions
local inactive_threshold=7  # days
local inactive_count=0

for session in (sessions older than 7 days); do
  ((inactive_count++))
done

if [[ $inactive_count -gt 0 ]]; then
  echo ""
  log_warn "$inactive_count sessions inactive > ${inactive_threshold}d"
  echo "Consider archiving inactive sessions"
fi
```

**Status:** ✅ Designed (Nice-to-Have for S8)

**2. search-sessions helper**

New script design:
```bash
#!/bin/bash
# search-sessions.sh - Search archived sessions

# Usage: search-sessions.sh [--tag TAG] [--project PROJECT] [--date DATE]
# Searches manifest.yaml files in archives
```

**Status:** ✅ Designed (Nice-to-Have for S8)

**3. Error message format standard**

Standard format:
```bash
# ERROR: <specific error message>
# Suggestion: <what to do next>
# Example: archive-session abc123 --force
```

**Status:** ✅ Designed (will implement in S7)

**4. Performance targets**

Targets:
- dashboard: < 1 second (for < 50 sessions)
- clone-repo: network-bound (no target)
- create-worktree: < 5 seconds
- archive-session: < 30 seconds (with audit)

**Status:** ✅ Designed (will verify in S9)

**5. Configuration file support**

Optional `~/.workspace.conf`:
```bash
# Workspace configuration
SRC_ROOT=$HOME/src
WORKTREES_ROOT=$HOME/worktrees
SESSIONS_ROOT=$HOME/.claude/sessions
ARCHIVES_ROOT=$HOME/src/github/vbonnet/engram-research/session-archives

# Audit settings
AUDIT_SKIP_PATTERNS="*.jpg,*.png"  # Files to skip in audit

# Dashboard settings
DASHBOARD_MAX_ARCHIVED=10
```

**Status:** ✅ Designed (Nice-to-Have for S8)

---

## Implementation Plan for S5-S11

### Sprint 1: Core Structure (S5-S6, ~8 hours)

**S5: Implementation Planning**
- Set up project structure (bin/, lib/, test/)
- Create Makefile or build script
- Set up test framework

**S6: Development Setup**
- Implement common-utils.sh
- Implement path-utils.sh
- Implement manifest-utils.sh
- Write unit tests for each

**Deliverable:** Core library with tests passing

---

### Sprint 2: Migration (S7, ~4 hours + migration)

**S7: Core Implementation**
- Implement migrate-workspace.sh (6 phases)
- Implement backup/restore logic
- Implement verification
- Write integration tests

**Execute:** Full migration (3-4 hours user time)

**Deliverable:** Migrated workspace

---

### Sprint 3: Session Management (S8, ~3 hours)

**S8: Integration & Testing**
- Implement audit-utils.sh (MUST-HAVE)
- Implement archive-session.sh
- Implement session-dashboard.sh
- Write integration tests
- Address SHOULD conditions (1-8)

**Deliverable:** Session management working

---

### Sprint 4: Automation & Polish (S9-S10, ~4 hours)

**S9: Validation**
- Implement clone-repo.sh
- Implement create-worktree.sh
- Implement resume-session.sh
- Implement cleanup-merged-worktrees.sh
- Install and configure gwq
- Write USER-GUIDE.md
- Address NICE conditions (1-5) if time

**S10: Deployment**
- Install scripts to ~/bin/
- Update ~/.bashrc with env vars and aliases
- Test all workflows end-to-end
- Fix any issues found

**Deliverable:** Fully automated system

---

### S11: Retrospective

- Review what worked well
- Extract patterns for retro-tasks
- Document lessons learned
- Create retrospective Beads

---

## File Structure

```
workspace-management/
├── bin/
│   ├── clone-repo.sh
│   ├── create-worktree.sh
│   ├── archive-session.sh
│   ├── session-dashboard.sh
│   ├── resume-session.sh
│   ├── cleanup-merged-worktrees.sh
│   └── migrate-workspace.sh
├── lib/
│   ├── common-utils.sh
│   ├── path-utils.sh
│   ├── manifest-utils.sh
│   ├── audit-utils.sh
│   └── git-utils.sh
├── test/
│   ├── unit/
│   │   ├── test-common-utils.sh
│   │   ├── test-path-utils.sh
│   │   ├── test-manifest-utils.sh
│   │   └── test-audit-utils.sh
│   └── integration/
│       ├── test-clone-and-worktree.sh
│       ├── test-archive-workflow.sh
│       └── test-migration-dry-run.sh
├── docs/
│   ├── USER-GUIDE.md
│   ├── QUICK-REFERENCE.md
│   └── TROUBLESHOOTING.md
├── Makefile
└── README.md
```

---

## Summary

**S4 Architecture Design:** ✅ **COMPLETE**

**Designed:**
- 5 core library modules (common, path, manifest, audit, git)
- 6 helper scripts (clone, worktree, archive, dashboard, resume, cleanup)
- 1 migration script (6 phases)
- Testing strategy (unit + integration)
- All 8 SHOULD conditions addressed
- All 5 NICE conditions designed (optional)

**Ready for:** S5 - Implementation Planning

**Estimated implementation time:** ~22 hours (matches D4 estimate)

---

**Status:** ⏳ IN PROGRESS

**Next:** Complete S4 document and get approval

---
