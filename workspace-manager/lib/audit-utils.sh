#!/usr/bin/env bash
#
# audit-utils.sh - Security audit and secret detection utilities
#
# Provides functions for detecting sensitive data and secrets in files
# Implements R2.3 requirement for comprehensive secret detection
#
# Usage:
#   source lib/audit-utils.sh
#
# Dependencies:
#   lib/common-utils.sh

# Source guard to prevent double-sourcing
[[ -n "${AUDIT_UTILS_LOADED:-}" ]] && return 0
readonly AUDIT_UTILS_LOADED=1

set -euo pipefail

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common-utils.sh"

#######################################
# Secret Patterns (7 types)
#######################################

# Pattern 1: Generic API keys (long alphanumeric strings)
readonly PATTERN_API_KEY='['\''"][A-Za-z0-9_-]{32,}['\''"]'

# Pattern 2: AWS credentials
readonly PATTERN_AWS='(AWS|aws)[ _-]?(SECRET|secret|ACCESS|access)?[ _-]?(KEY|key|ID|id)[ ]*[:=][ ]*['\''"][A-Za-z0-9/+=]{16,}['\''"]'

# Pattern 3: Private keys (RSA, SSH, PGP)
readonly PATTERN_PRIVATE_KEY='-----BEGIN (RSA |DSA |EC |OPENSSH |PGP )?PRIVATE KEY-----'

# Pattern 4: Generic tokens (Bearer, JWT, etc.)
readonly PATTERN_TOKEN='(token|TOKEN|bearer|BEARER|jwt|JWT)[ ]*[:=][ ]*['\''"][A-Za-z0-9_\.\-]{20,}['\''"]'

# Pattern 5: Passwords in configuration
readonly PATTERN_PASSWORD='(password|PASSWORD|passwd|PASSWD|pwd|PWD)[ ]*[:=][ ]*['\''"][^'\''"\n]{8,}['\''"]'

# Pattern 6: SSH private key files
readonly PATTERN_SSH_KEY='id_(rsa|dsa|ecdsa|ed25519)$|\.pem$|\.key$'

# Pattern 7: Database connection strings
readonly PATTERN_DB_URL='(mysql|postgresql|mongodb|redis)://[^:]+:[^@]+@'

#######################################
# File Scanning Functions
#######################################

# Scan a single file for secrets
# Arguments:
#   $1 - File path to scan
# Outputs:
#   Lines containing potential secrets (with line numbers)
# Returns:
#   0 if no secrets found, 1 if secrets detected
scan_file_for_secrets() {
  local file="$1"

  validate_file "$file" "File to scan"

  log_debug "Scanning file for secrets: $file"

  local findings=()
  local found=0

  # Skip binary files
  if file "$file" | grep -q "binary"; then
    log_debug "Skipping binary file: $file"
    return 0
  fi

  # Pattern 1: API keys
  if grep -n -E "$PATTERN_API_KEY" "$file" 2>/dev/null; then
    findings+=("API Key pattern detected")
    found=1
  fi

  # Pattern 2: AWS credentials
  if grep -n -E "$PATTERN_AWS" "$file" 2>/dev/null; then
    findings+=("AWS credential pattern detected")
    found=1
  fi

  # Pattern 3: Private keys
  if grep -n -E "$PATTERN_PRIVATE_KEY" "$file" 2>/dev/null; then
    findings+=("Private key detected")
    found=1
  fi

  # Pattern 4: Tokens
  if grep -n -E "$PATTERN_TOKEN" "$file" 2>/dev/null; then
    findings+=("Token pattern detected")
    found=1
  fi

  # Pattern 5: Passwords
  if grep -n -E "$PATTERN_PASSWORD" "$file" 2>/dev/null; then
    findings+=("Password pattern detected")
    found=1
  fi

  # Pattern 6: SSH key files (by filename)
  if [[ "$(basename "$file")" =~ $PATTERN_SSH_KEY ]]; then
    echo "SSH private key file: $file"
    findings+=("SSH key file detected")
    found=1
  fi

  # Pattern 7: Database URLs
  if grep -n -E "$PATTERN_DB_URL" "$file" 2>/dev/null; then
    findings+=("Database connection string detected")
    found=1
  fi

  return $found
}

# Scan a directory recursively for secrets
# Arguments:
#   $1 - Directory path to scan
#   $2 - Max depth (optional, default: unlimited)
# Outputs:
#   List of files containing potential secrets
# Returns:
#   0 if no secrets found, 1 if secrets detected
scan_directory_for_secrets() {
  local dir="$1"
  local max_depth="${2:--1}"

  validate_directory "$dir" "Directory to scan"

  log_debug "Scanning directory for secrets: $dir (max depth: $max_depth)"

  local found=0
  local files_scanned=0
  local files_with_secrets=0

  # Build find command with optional max depth
  local find_cmd="find \"$dir\" -type f"
  if [[ $max_depth -gt 0 ]]; then
    find_cmd="$find_cmd -maxdepth $max_depth"
  fi

  # Scan each file
  while IFS= read -r file; do
    ((files_scanned++))

    # Skip certain file types
    local basename
    basename="$(basename "$file")"

    # Skip hidden files starting with .
    if [[ "$basename" =~ ^\. ]]; then
      continue
    fi

    # Scan file
    if scan_file_for_secrets "$file" >/dev/null 2>&1; then
      echo "⚠️  $file"
      ((files_with_secrets++))
      found=1
    fi
  done < <(eval "$find_cmd" 2>/dev/null)

  log_debug "Scanned $files_scanned files, found secrets in $files_with_secrets files"

  return $found
}

#######################################
# Session Auditing (R2.3)
#######################################

# Audit a session for sensitive data
# Scans: manifest.yaml + working/ + artifacts/
# Arguments:
#   $1 - Session directory path
# Outputs:
#   Report of findings
# Returns:
#   0 if no secrets found, 1 if secrets detected
audit_session_for_secrets() {
  local session_dir="$1"

  validate_directory "$session_dir" "Session directory"

  log_info "Auditing session for sensitive data: $session_dir"

  local found=0
  local findings=()

  # Audit 1: manifest.yaml
  local manifest="$session_dir/manifest.yaml"
  if [[ -f "$manifest" ]]; then
    log_debug "Auditing manifest.yaml"
    if scan_file_for_secrets "$manifest" >/dev/null 2>&1; then
      findings+=("manifest.yaml contains potential secrets")
      echo "⚠️  Secrets detected in: manifest.yaml"
      scan_file_for_secrets "$manifest" 2>/dev/null | sed 's/^/    /'
      found=1
    else
      echo "✅ manifest.yaml: clean"
    fi
  fi

  # Audit 2: working/ directory
  local working_dir="$session_dir/working"
  if [[ -d "$working_dir" ]]; then
    log_debug "Auditing working/ directory"
    echo ""
    echo "Scanning working/ directory..."

    if scan_directory_for_secrets "$working_dir" 3 2>/dev/null | grep "^⚠️"; then
      findings+=("working/ directory contains potential secrets")
      found=1
    else
      echo "✅ working/ directory: clean"
    fi
  fi

  # Audit 3: artifacts/ directory
  local artifacts_dir="$session_dir/artifacts"
  if [[ -d "$artifacts_dir" ]]; then
    log_debug "Auditing artifacts/ directory"
    echo ""
    echo "Scanning artifacts/ directory..."

    if scan_directory_for_secrets "$artifacts_dir" 2 2>/dev/null | grep "^⚠️"; then
      findings+=("artifacts/ directory contains potential secrets")
      found=1
    else
      echo "✅ artifacts/ directory: clean"
    fi
  fi

  # Summary
  echo ""
  if [[ $found -eq 0 ]]; then
    log_success "Audit complete: No sensitive data detected"
  else
    log_warn "Audit complete: ${#findings[@]} area(s) with potential secrets"
    for finding in "${findings[@]}"; do
      echo "  - $finding"
    done
  fi

  return $found
}

#######################################
# Interactive Audit with Confirmation
#######################################

# Audit session and prompt user for confirmation to proceed
# Arguments:
#   $1 - Session directory path
#   $2 - Operation description (e.g., "archive this session")
# Returns:
#   0 if user confirms to proceed, 1 if user cancels
audit_and_confirm() {
  local session_dir="$1"
  local operation="${2:-proceed}"

  log_info "Running security audit before ${operation}..."
  echo ""

  # Run audit
  local audit_result=0
  audit_session_for_secrets "$session_dir" || audit_result=$?

  echo ""

  # If secrets found, prompt for confirmation
  if [[ $audit_result -ne 0 ]]; then
    log_warn "Potential secrets detected in session!"
    echo ""
    echo "⚠️  WARNING: This session may contain sensitive data."
    echo "   Please review the findings above before proceeding."
    echo ""

    if confirm "Do you want to $operation anyway?" "n"; then
      log_info "User confirmed to proceed despite findings"
      return 0
    else
      log_info "User cancelled operation"
      return 1
    fi
  else
    log_success "No sensitive data detected. Safe to proceed."
    return 0
  fi
}

#######################################
# Pattern Testing (for validation)
#######################################

# Test secret patterns against sample inputs
# Useful for verifying patterns work as expected
# Outputs:
#   Test results for each pattern
# Returns:
#   0 on success
test_secret_patterns() {
  echo "Testing secret detection patterns..."
  echo ""

  # Test 1: API Key
  echo "Test 1: API Key pattern"
  echo 'api_key="sk_live_51234567890abcdefghijklmnopqrstuvwxyz"' | grep -E "$PATTERN_API_KEY" && echo "✅ Detected" || echo "❌ Not detected"
  echo ""

  # Test 2: AWS credentials
  echo "Test 2: AWS credentials pattern"
  echo 'AWS_SECRET_ACCESS_KEY="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"' | grep -E "$PATTERN_AWS" && echo "✅ Detected" || echo "❌ Not detected"
  echo ""

  # Test 3: Private key
  echo "Test 3: Private key pattern"
  echo '-----BEGIN RSA PRIVATE KEY-----' | grep -E "$PATTERN_PRIVATE_KEY" && echo "✅ Detected" || echo "❌ Not detected"
  echo ""

  # Test 4: Token
  echo "Test 4: Token pattern"
  echo 'bearer_token="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"' | grep -E "$PATTERN_TOKEN" && echo "✅ Detected" || echo "❌ Not detected"
  echo ""

  # Test 5: Password
  echo "Test 5: Password pattern"
  echo 'database_password="MyS3cr3tP@ssw0rd!"' | grep -E "$PATTERN_PASSWORD" && echo "✅ Detected" || echo "❌ Not detected"
  echo ""

  # Test 6: SSH key filename
  echo "Test 6: SSH key filename pattern"
  echo 'id_rsa' | grep -E "$PATTERN_SSH_KEY" && echo "✅ Detected" || echo "❌ Not detected"
  echo ""

  # Test 7: Database URL
  echo "Test 7: Database URL pattern"
  echo 'DATABASE_URL="postgresql://user:password@localhost:5432/mydb"' | grep -E "$PATTERN_DB_URL" && echo "✅ Detected" || echo "❌ Not detected"
  echo ""

  echo "Pattern testing complete"

  return 0
}

# Export functions for use in other scripts
export -f scan_file_for_secrets scan_directory_for_secrets
export -f audit_session_for_secrets audit_and_confirm
export -f test_secret_patterns
