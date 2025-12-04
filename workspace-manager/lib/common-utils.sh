#!/usr/bin/env bash
#
# common-utils.sh - Common utility functions for workspace management
#
# Provides logging, error handling, validation, and user interaction utilities
# that are used across all scripts in the workspace management system.
#
# Usage:
#   source lib/common-utils.sh
#
# Environment Variables:
#   DEBUG - Set to "1" to enable debug logging
#   COLOR - Set to "0" to disable colored output

# Source guard to prevent double-sourcing
[[ -n "${COMMON_UTILS_LOADED:-}" ]] && return 0
readonly COMMON_UTILS_LOADED=1

set -euo pipefail

# Color codes for output (disable with COLOR=0)
if [[ "${COLOR:-1}" == "1" ]]; then
  readonly COLOR_RESET='\033[0m'
  readonly COLOR_RED='\033[0;31m'
  readonly COLOR_GREEN='\033[0;32m'
  readonly COLOR_YELLOW='\033[0;33m'
  readonly COLOR_BLUE='\033[0;34m'
  readonly COLOR_GRAY='\033[0;90m'
else
  readonly COLOR_RESET=''
  readonly COLOR_RED=''
  readonly COLOR_GREEN=''
  readonly COLOR_YELLOW=''
  readonly COLOR_BLUE=''
  readonly COLOR_GRAY=''
fi

#######################################
# Logging Functions
#######################################

# Log an informational message
# Arguments:
#   $1 - Message to log
# Outputs:
#   Writes message to stdout in blue
log_info() {
  local message="$1"
  echo -e "${COLOR_BLUE}INFO:${COLOR_RESET} ${message}"
}

# Log an error message
# Arguments:
#   $1 - Error message to log
# Outputs:
#   Writes message to stderr in red
log_error() {
  local message="$1"
  echo -e "${COLOR_RED}ERROR:${COLOR_RESET} ${message}" >&2
}

# Log a success message
# Arguments:
#   $1 - Success message to log
# Outputs:
#   Writes message to stdout in green
log_success() {
  local message="$1"
  echo -e "${COLOR_GREEN}SUCCESS:${COLOR_RESET} ${message}"
}

# Log a warning message
# Arguments:
#   $1 - Warning message to log
# Outputs:
#   Writes message to stderr in yellow
log_warn() {
  local message="$1"
  echo -e "${COLOR_YELLOW}WARNING:${COLOR_RESET} ${message}" >&2
}

# Log a debug message (only if DEBUG=1)
# Arguments:
#   $1 - Debug message to log
# Outputs:
#   Writes message to stdout in gray if DEBUG=1
log_debug() {
  if [[ "${DEBUG:-0}" == "1" ]]; then
    local message="$1"
    echo -e "${COLOR_GRAY}DEBUG:${COLOR_RESET} ${message}"
  fi
}

#######################################
# Error Handling
#######################################

# Exit with an error message
# Arguments:
#   $1 - Error message
#   $2 - Exit code (optional, default: 1)
# Returns:
#   Never returns (exits the script)
error_exit() {
  local message="$1"
  local exit_code="${2:-1}"

  log_error "$message"
  exit "$exit_code"
}

# Validate that a file exists and is readable
# Arguments:
#   $1 - File path
#   $2 - Human-readable file description (optional, default: "File")
# Returns:
#   0 if file exists and is readable
#   Exits with error otherwise
validate_file() {
  local file="$1"
  local description="${2:-File}"

  log_debug "Validating file: $file"

  if [[ ! -f "$file" ]]; then
    error_exit "$description not found: $file"
  fi

  if [[ ! -r "$file" ]]; then
    error_exit "$description not readable: $file"
  fi

  return 0
}

# Validate that a directory exists and is accessible
# Arguments:
#   $1 - Directory path
#   $2 - Human-readable directory description (optional, default: "Directory")
# Returns:
#   0 if directory exists and is accessible
#   Exits with error otherwise
validate_directory() {
  local dir="$1"
  local description="${2:-Directory}"

  log_debug "Validating directory: $dir"

  if [[ ! -d "$dir" ]]; then
    error_exit "$description not found: $dir"
  fi

  if [[ ! -r "$dir" || ! -x "$dir" ]]; then
    error_exit "$description not accessible: $dir"
  fi

  return 0
}

# Validate that a string is not empty
# Arguments:
#   $1 - String to validate
#   $2 - Human-readable variable name for error messages
# Returns:
#   0 if string is non-empty
#   Exits with error otherwise
validate_not_empty() {
  local value="$1"
  local name="$2"

  if [[ -z "$value" ]]; then
    error_exit "$name cannot be empty"
  fi

  return 0
}

# Validate that required environment variables are set
# Arguments:
#   $@ - List of environment variable names
# Returns:
#   0 if all variables are set
#   Exits with error listing missing variables
check_env_vars() {
  local missing_vars=()

  for var in "$@"; do
    if [[ -z "${!var:-}" ]]; then
      missing_vars+=("$var")
    fi
  done

  if [[ ${#missing_vars[@]} -gt 0 ]]; then
    error_exit "Missing required environment variables: ${missing_vars[*]}"
  fi

  return 0
}

# Validate that a path length is within acceptable limits
# Arguments:
#   $1 - Path to validate
#   $2 - Human-readable name for error messages (optional, default: "Path")
#   $3 - Maximum length in bytes (optional, default: 4000)
# Returns:
#   0 if path length is acceptable
#   Exits with error and helpful suggestion otherwise
validate_path_length() {
  local path="$1"
  local name="${2:-Path}"
  local max_length="${3:-4000}"

  log_debug "Validating path length: ${#path} bytes (max: $max_length)"

  if [[ ${#path} -gt $max_length ]]; then
    error_exit "$name is too long (${#path} chars, max ${max_length}):
  $path

Suggestion: Use shorter branch names or shallower directory hierarchy
Example: 'feature-auth' instead of 'feature/user-authentication-system-with-oauth'"
  fi

  return 0
}

#######################################
# User Interaction
#######################################

# Prompt user for yes/no confirmation
# Arguments:
#   $1 - Prompt message
#   $2 - Default answer (optional: "y" or "n", default: none)
# Returns:
#   0 if user confirmed (yes)
#   1 if user declined (no)
confirm() {
  local prompt="$1"
  local default="${2:-}"
  local response

  # Build prompt suffix based on default
  local prompt_suffix
  if [[ "$default" == "y" ]]; then
    prompt_suffix=" [Y/n]"
  elif [[ "$default" == "n" ]]; then
    prompt_suffix=" [y/N]"
  else
    prompt_suffix=" [y/n]"
  fi

  # Prompt user
  echo -n "$prompt$prompt_suffix "
  read -r response

  # Handle empty response with default
  if [[ -z "$response" && -n "$default" ]]; then
    response="$default"
  fi

  # Check response
  case "${response,,}" in
    y|yes)
      return 0
      ;;
    n|no)
      return 1
      ;;
    *)
      log_error "Invalid response. Please answer 'y' or 'n'."
      confirm "$prompt" "$default"
      return $?
      ;;
  esac
}

#######################################
# Time and Date Utilities
#######################################

# Format a timestamp as "X time ago" relative to now
# Arguments:
#   $1 - ISO 8601 timestamp (YYYY-MM-DDTHH:MM:SSZ)
# Outputs:
#   Human-readable relative time string
# Returns:
#   0 on success, 1 on invalid timestamp
format_time_ago() {
  local timestamp="$1"

  # Parse timestamp to epoch seconds
  local timestamp_epoch
  timestamp_epoch=$(date -d "$timestamp" +%s 2>/dev/null) || {
    echo "invalid timestamp"
    return 1
  }

  # Get current time
  local now
  now=$(date +%s)

  # Calculate difference
  local diff=$((now - timestamp_epoch))

  # Format based on magnitude
  if [[ $diff -lt 60 ]]; then
    echo "just now"
  elif [[ $diff -lt 3600 ]]; then
    local minutes=$((diff / 60))
    if [[ $minutes -eq 1 ]]; then
      echo "1 minute ago"
    else
      echo "${minutes} minutes ago"
    fi
  elif [[ $diff -lt 86400 ]]; then
    local hours=$((diff / 3600))
    if [[ $hours -eq 1 ]]; then
      echo "1 hour ago"
    else
      echo "${hours} hours ago"
    fi
  elif [[ $diff -lt 604800 ]]; then
    local days=$((diff / 86400))
    if [[ $days -eq 1 ]]; then
      echo "1 day ago"
    else
      echo "${days} days ago"
    fi
  elif [[ $diff -lt 2592000 ]]; then
    local weeks=$((diff / 604800))
    if [[ $weeks -eq 1 ]]; then
      echo "1 week ago"
    else
      echo "${weeks} weeks ago"
    fi
  else
    local months=$((diff / 2592000))
    if [[ $months -eq 1 ]]; then
      echo "1 month ago"
    else
      echo "${months} months ago"
    fi
  fi

  return 0
}

# Export functions for use in other scripts
# Note: This is only needed if scripts use 'bash -c' to call these functions
# For normal sourcing, this is not required
export -f log_info log_error log_success log_warn log_debug
export -f error_exit validate_file validate_directory validate_not_empty
export -f check_env_vars validate_path_length confirm format_time_ago
