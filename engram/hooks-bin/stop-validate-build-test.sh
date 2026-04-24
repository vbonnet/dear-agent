#!/bin/bash
# Stop hook: Validate build/test integrity before session exit
# This hook runs automatically when Claude tries to exit a session
#
# Input: JSON from stdin (transcript_path, session_id, timestamp)
# Output: Validation results to stderr
# Exit code: 0 = success (safe to exit), 1 = failure (show warning)
#
# Note: Stop hooks cannot technically block exit (only PreToolUse hooks can block).
# This hook provides loud feedback. Always-loaded engram enforces behavioral compliance.

set -euo pipefail

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source shared validation library
if [ -f "$SCRIPT_DIR/lib/validate_build_test.sh" ]; then
    # shellcheck source=lib/validate_build_test.sh
    source "$SCRIPT_DIR/lib/validate_build_test.sh"
else
    echo "❌ Error: validate_build_test.sh library not found" >&2
    echo "Expected location: $SCRIPT_DIR/lib/validate_build_test.sh" >&2
    exit 1
fi

# Colors (redeclare in case sourcing failed)
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

#######################################
# Main validation flow
#######################################
main() {
    # Read JSON input from stdin (provided by Claude Code Stop hook system)
    local input
    input=$(cat)

    # Extract transcript_path (optional, for future use)
    local transcript_path
    transcript_path=$(echo "$input" | jq -r '.transcript_path // ""' 2>/dev/null || echo "")

    # Load configuration
    local config
    config=$(load_config)

    # Check if enabled
    local enabled
    enabled=$(echo "$config" | jq -r '.enabled')
    if [ "$enabled" = "false" ]; then
        echo "ℹ️  Build/test integrity checks disabled in .build-integrity.yaml" >&2
        exit 0
    fi

    # Get timeout
    local timeout
    timeout=$(echo "$config" | jq -r '.timeout')

    # Detect test command
    local test_cmd
    test_cmd=$(detect_test_command "$config")

    # Detect build command
    local build_cmd
    build_cmd=$(detect_build_command "$config")

    # Check if any validation to run
    if [ -z "$test_cmd" ] && [ -z "$build_cmd" ]; then
        echo "ℹ️  No test or build commands detected (skipping validation)" >&2
        exit 0
    fi

    echo "" >&2
    echo "🔍 Running build/test integrity check..." >&2
    echo "" >&2

    # Run tests
    local tests_passed="true"
    local tests_output=""
    if [ -n "$test_cmd" ]; then
        echo "Running tests: $test_cmd" >&2
        if tests_output=$(run_tests "$test_cmd" "$timeout" 2>&1); then
            tests_passed="true"
        else
            tests_passed="false"
        fi
    fi

    # Run build (skip if tests failed)
    local build_passed="true"
    local build_output=""
    if [ -n "$build_cmd" ]; then
        if [ "$tests_passed" = "false" ]; then
            build_passed="skipped"
            build_output="Skipped (tests failed)"
        else
            echo "Running build: $build_cmd" >&2
            if build_output=$(run_build "$build_cmd" "$timeout" 2>&1); then
                build_passed="true"
            else
                build_passed="false"
            fi
        fi
    fi

    # Format and display results
    format_results "$tests_passed" "$tests_output" "$build_passed" "$build_output" "false" >&2

    # Show remediation if failed
    if [ "$tests_passed" = "false" ] || [ "$build_passed" = "false" ]; then
        show_remediation >&2
        exit 1  # Non-zero exit = show error feedback (doesn't block exit)
    fi

    # Success
    echo -e "${GREEN}✅ All checks passed. Safe to exit session.${NC}" >&2
    exit 0
}

# Run main function
main
