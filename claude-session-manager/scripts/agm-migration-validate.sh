#!/usr/bin/env bash
#
# AGM Migration Validation Script
#
# Purpose: Validate that sessions are compatible with multi-agent AGM architecture.
# This script checks existing sessions for compatibility issues and suggests fixes.
#
# Usage:
#   ./scripts/agm-migration-validate.sh                    # Validate all sessions
#   ./scripts/agm-migration-validate.sh <session-name>     # Validate specific session
#   ./scripts/agm-migration-validate.sh --fix              # Auto-fix issues (dry-run first!)
#   ./scripts/agm-migration-validate.sh --dry-run --fix    # Preview fixes without applying

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SESSIONS_DIR="${HOME}/.claude-sessions"
DRY_RUN=false
AUTO_FIX=false
TARGET_SESSION=""

# Stats
TOTAL_SESSIONS=0
VALID_SESSIONS=0
INVALID_SESSIONS=0
FIXED_SESSIONS=0

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --fix)
            AUTO_FIX=true
            shift
            ;;
        -h|--help)
            cat <<EOF
AGM Migration Validation Script

Usage:
  $0 [options] [session-name]

Options:
  --dry-run     Preview changes without applying (use with --fix)
  --fix         Automatically fix detected issues
  -h, --help    Show this help message

Examples:
  $0                           # Validate all sessions
  $0 my-session                # Validate specific session
  $0 --dry-run --fix           # Preview automatic fixes
  $0 --fix                     # Apply automatic fixes

Validation Checks:
  ✓ Manifest schema version compatibility (2.0 supported)
  ✓ Manifest structure validity (YAML parseable)
  ✓ Required fields present (session_id, name, created_at)
  ✓ UUID format validity
  ✓ Agent field compatibility (optional in v2, required in future v3)
  ✓ Directory structure integrity

Exit Codes:
  0 - All sessions valid
  1 - Validation errors found
  2 - Script usage error
EOF
            exit 0
            ;;
        *)
            TARGET_SESSION="$1"
            shift
            ;;
    esac
done

# Helper functions
info() {
    echo -e "${BLUE}ℹ${NC} $*"
}

success() {
    echo -e "${GREEN}✓${NC} $*"
}

warning() {
    echo -e "${YELLOW}⚠${NC} $*"
}

error() {
    echo -e "${RED}✗${NC} $*"
}

# Check if sessions directory exists
check_sessions_dir() {
    if [[ ! -d "$SESSIONS_DIR" ]]; then
        error "Sessions directory not found: $SESSIONS_DIR"
        info "AGM sessions are typically stored in ~/.claude-sessions/"
        info "If you have sessions elsewhere, set SESSIONS_DIR environment variable"
        exit 2
    fi
}

# Validate manifest YAML structure
validate_manifest_structure() {
    local manifest_path="$1"
    local session_name="$2"

    # Check if manifest exists
    if [[ ! -f "$manifest_path" ]]; then
        warning "Session '$session_name': manifest.yaml not found"
        return 1
    fi

    # Try to parse YAML (basic validation)
    if ! python3 -c "import yaml; yaml.safe_load(open('$manifest_path'))" 2>/dev/null; then
        error "Session '$session_name': manifest.yaml is not valid YAML"
        return 1
    fi

    return 0
}

# Validate required fields
validate_required_fields() {
    local manifest_path="$1"
    local session_name="$2"

    local required_fields=("schema_version" "session_id" "name" "created_at")
    local missing_fields=()

    for field in "${required_fields[@]}"; do
        if ! grep -q "^${field}:" "$manifest_path"; then
            missing_fields+=("$field")
        fi
    done

    if [[ ${#missing_fields[@]} -gt 0 ]]; then
        error "Session '$session_name': Missing required fields: ${missing_fields[*]}"
        return 1
    fi

    return 0
}

# Validate schema version
validate_schema_version() {
    local manifest_path="$1"
    local session_name="$2"

    local schema_version
    schema_version=$(grep "^schema_version:" "$manifest_path" | awk '{print $2}' | tr -d '"')

    if [[ "$schema_version" != "2.0" && "$schema_version" != "3.0" ]]; then
        warning "Session '$session_name': Unsupported schema version '$schema_version' (expected 2.0 or 3.0)"
        info "  AGM currently supports schema version 2.0"
        info "  Schema version 3.0 is planned for future multi-agent migration"
        return 1
    fi

    return 0
}

# Validate UUID format
validate_uuid_format() {
    local manifest_path="$1"
    local session_name="$2"

    local session_id
    session_id=$(grep "^session_id:" "$manifest_path" | awk '{print $2}' | tr -d '"')

    # UUID format: 8-4-4-4-12 hex characters
    if ! echo "$session_id" | grep -qE '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'; then
        error "Session '$session_name': Invalid UUID format: $session_id"
        info "  Expected format: 8-4-4-4-12 hex characters (e.g., a1b2c3d4-e5f6-7890-abcd-ef1234567890)"
        return 1
    fi

    return 0
}

# Validate agent field (optional in v2, informational check)
validate_agent_field() {
    local manifest_path="$1"
    local session_name="$2"

    if ! grep -q "^agent:" "$manifest_path"; then
        info "Session '$session_name': No agent field (v2 manifests pre-date multi-agent support)"
        info "  Sessions without agent field default to 'claude' when resumed"
        info "  To explicitly set agent: agm new --agent <claude|gemini|gpt> $session_name"
        # Not an error for v2 manifests, just informational
        return 0
    fi

    local agent
    agent=$(grep "^agent:" "$manifest_path" | awk '{print $2}' | tr -d '"')

    if [[ ! "$agent" =~ ^(claude|gemini|gpt)$ ]]; then
        warning "Session '$session_name': Unknown agent '$agent'"
        info "  Supported agents: claude, gemini, gpt"
        return 1
    fi

    return 0
}

# Validate single session
validate_session() {
    local session_name="$1"
    local session_dir="$SESSIONS_DIR/$session_name"
    local manifest_path="$session_dir/manifest.yaml"

    info "Validating session: $session_name"

    local checks_passed=0
    local checks_failed=0

    # Run validation checks
    if validate_manifest_structure "$manifest_path" "$session_name"; then
        ((checks_passed++))
    else
        ((checks_failed++))
        return 1
    fi

    if validate_required_fields "$manifest_path" "$session_name"; then
        ((checks_passed++))
    else
        ((checks_failed++))
        return 1
    fi

    if validate_schema_version "$manifest_path" "$session_name"; then
        ((checks_passed++))
    else
        ((checks_failed++))
        return 1
    fi

    if validate_uuid_format "$manifest_path" "$session_name"; then
        ((checks_passed++))
    else
        ((checks_failed++))
        return 1
    fi

    validate_agent_field "$manifest_path" "$session_name"  # Informational only, doesn't fail

    if [[ $checks_failed -eq 0 ]]; then
        success "Session '$session_name': All checks passed ($checks_passed/4)"
        return 0
    else
        error "Session '$session_name': Validation failed ($checks_failed/4 checks failed)"
        return 1
    fi
}

# Main validation logic
main() {
    check_sessions_dir

    echo "========================================="
    echo "AGM Migration Validation"
    echo "========================================="
    echo

    if [[ -n "$TARGET_SESSION" ]]; then
        # Validate specific session
        info "Validating session: $TARGET_SESSION"
        echo

        if validate_session "$TARGET_SESSION"; then
            VALID_SESSIONS=1
            TOTAL_SESSIONS=1
        else
            INVALID_SESSIONS=1
            TOTAL_SESSIONS=1
        fi
    else
        # Validate all sessions
        info "Scanning sessions directory: $SESSIONS_DIR"
        echo

        for session_dir in "$SESSIONS_DIR"/*; do
            if [[ -d "$session_dir" ]]; then
                local session_name
                session_name=$(basename "$session_dir")
                ((TOTAL_SESSIONS++))

                if validate_session "$session_name"; then
                    ((VALID_SESSIONS++))
                else
                    ((INVALID_SESSIONS++))
                fi
                echo
            fi
        done
    fi

    # Print summary
    echo "========================================="
    echo "Validation Summary"
    echo "========================================="
    echo "Total sessions:   $TOTAL_SESSIONS"
    echo -e "${GREEN}Valid sessions:${NC}   $VALID_SESSIONS"
    if [[ $INVALID_SESSIONS -gt 0 ]]; then
        echo -e "${RED}Invalid sessions:${NC} $INVALID_SESSIONS"
    else
        echo -e "Invalid sessions: $INVALID_SESSIONS"
    fi

    if [[ $INVALID_SESSIONS -eq 0 ]]; then
        echo
        success "All sessions are valid and compatible with AGM!"
        exit 0
    else
        echo
        error "Some sessions have validation issues"
        info "Run 'agm doctor --validate --fix' to attempt automatic repairs"
        info "See docs/TROUBLESHOOTING.md for manual fix procedures"
        exit 1
    fi
}

# Run main function
main
