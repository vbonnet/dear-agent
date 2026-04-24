#!/usr/bin/env bash
#
# verify-cli-docs.sh - CLI Documentation Verification Script
#
# Purpose: Verify CLI tool documentation matches actual implementation
#
# Usage: ./verify-cli-docs.sh [OPTIONS]
#
# Exit Codes:
#   0 - All documentation synced with implementation
#   1 - Drift detected (documented flags don't match actual flags)
#   2 - Error (missing tools, invalid paths, etc.)
#
# Author: cli-docs-sync-audit swarm
# Created: 2026-02-04

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
DOCS_ROOT="${DOCS_ROOT:-$HOME/.claude/plugins/cache}"
VERBOSE="${VERBOSE:-false}"
TOOLS_TO_CHECK=()
DRIFT_DETECTED=false

# Usage information
usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Verify CLI tool documentation matches actual implementation by:
1. Extracting documented flags from markdown files
2. Running tools with --help to get actual flags
3. Comparing and reporting discrepancies

Options:
  -h, --help          Show this help message
  -v, --verbose       Verbose output
  -d, --docs-root DIR Documentation root directory (default: ~/.claude/plugins/cache)
  -t, --tool TOOL     Check specific tool only (can be repeated)

Examples:
  # Check all tools
  $0

  # Check specific tool
  $0 --tool csm

  # Verbose mode
  $0 --verbose

  # Custom docs location
  $0 --docs-root ~/my-docs

Exit Codes:
  0 - Documentation synced
  1 - Drift detected
  2 - Error (missing tool, invalid path, etc.)

EOF
    exit 0
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -d|--docs-root)
                DOCS_ROOT="$2"
                shift 2
                ;;
            -t|--tool)
                TOOLS_TO_CHECK+=("$2")
                shift 2
                ;;
            *)
                echo "Error: Unknown option $1" >&2
                echo "Run '$0 --help' for usage" >&2
                exit 2
                ;;
        esac
    done
}

# Log message with color
log() {
    local level=$1
    shift
    local message="$*"

    case $level in
        INFO)
            echo -e "${BLUE}[INFO]${NC} $message"
            ;;
        SUCCESS)
            echo -e "${GREEN}[✓]${NC} $message"
            ;;
        WARNING)
            echo -e "${YELLOW}[!]${NC} $message"
            ;;
        ERROR)
            echo -e "${RED}[✗]${NC} $message"
            ;;
    esac
}

# Verbose log (only if VERBOSE=true)
vlog() {
    if [[ "$VERBOSE" == "true" ]]; then
        log INFO "$@"
    fi
}

# Extract flags from markdown documentation
# Looks for patterns like: --flag, --flag-name, -f
extract_documented_flags() {
    local doc_file=$1
    local tool_name=$2

    vlog "Extracting flags from $doc_file"

    # Extract lines that look like flag documentation
    # Patterns: --flag, --flag-name VALUE, -f
    grep -oE -- '--[a-z0-9-]+|-[a-z]' "$doc_file" 2>/dev/null | sort -u || true
}

# Get actual flags from tool's --help output
get_actual_flags() {
    local tool_name=$1

    vlog "Running $tool_name --help"

    # Try to run tool with --help
    if ! command -v "$tool_name" &>/dev/null; then
        log WARNING "Tool '$tool_name' not found in PATH"
        return 1
    fi

    # Run --help and extract flags
    # Most tools output flags in format: --flag-name, -f
    "$tool_name" --help 2>&1 | grep -oE -- '--[a-z0-9-]+|-[a-z]' | sort -u || true
}

# Compare documented vs actual flags
compare_flags() {
    local tool_name=$1
    local doc_file=$2

    log INFO "Checking $tool_name"

    # Get documented flags
    local documented_flags
    documented_flags=$(extract_documented_flags "$doc_file" "$tool_name")

    if [[ -z "$documented_flags" ]]; then
        vlog "No flags documented for $tool_name"
        return 0
    fi

    # Get actual flags
    local actual_flags
    actual_flags=$(get_actual_flags "$tool_name")

    if [[ $? -ne 0 ]]; then
        log WARNING "Skipping $tool_name (tool not available)"
        return 0
    fi

    if [[ -z "$actual_flags" ]]; then
        log WARNING "$tool_name --help returned no flags"
        return 0
    fi

    # Find flags in docs but not in implementation
    local missing_flags
    missing_flags=$(comm -23 <(echo "$documented_flags") <(echo "$actual_flags"))

    # Find flags in implementation but not in docs
    local undocumented_flags
    undocumented_flags=$(comm -13 <(echo "$documented_flags") <(echo "$actual_flags"))

    # Report findings
    if [[ -n "$missing_flags" ]]; then
        log ERROR "Documented but missing in $tool_name:"
        echo "$missing_flags" | while read -r flag; do
            echo "    $flag"
        done
        DRIFT_DETECTED=true
    fi

    if [[ -n "$undocumented_flags" ]]; then
        log WARNING "Exists in $tool_name but not documented:"
        echo "$undocumented_flags" | while read -r flag; do
            echo "    $flag"
        done
    fi

    if [[ -z "$missing_flags" && -z "$undocumented_flags" ]]; then
        log SUCCESS "$tool_name documentation is synced"
    fi
}

# Find all documentation files for tools
find_tool_docs() {
    if [[ ! -d "$DOCS_ROOT" ]]; then
        log ERROR "Documentation root not found: $DOCS_ROOT"
        exit 2
    fi

    # Find all .md files in commands directories
    find "$DOCS_ROOT" -type f -name "*.md" -path "*/commands/*" 2>/dev/null || true
}

# Main verification logic
main() {
    parse_args "$@"

    log INFO "CLI Documentation Verification"
    log INFO "Docs root: $DOCS_ROOT"
    echo

    # Determine which tools to check
    local doc_files
    if [[ ${#TOOLS_TO_CHECK[@]} -eq 0 ]]; then
        # Check all tools
        vlog "Scanning for all tool documentation..."
        doc_files=$(find_tool_docs)
    else
        # Check specific tools
        doc_files=""
        for tool in "${TOOLS_TO_CHECK[@]}"; do
            local tool_docs
            tool_docs=$(find "$DOCS_ROOT" -type f -name "${tool}*.md" -path "*/commands/*" 2>/dev/null || true)
            doc_files="$doc_files"$'\n'"$tool_docs"
        done
    fi

    if [[ -z "$doc_files" ]]; then
        log WARNING "No documentation files found"
        exit 0
    fi

    # Process each documentation file
    echo "$doc_files" | while IFS= read -r doc_file; do
        [[ -z "$doc_file" ]] && continue

        # Extract tool name from filename (e.g., csm-assoc.md -> csm)
        local filename
        filename=$(basename "$doc_file" .md)
        local tool_name
        tool_name=$(echo "$filename" | cut -d'-' -f1)

        compare_flags "$tool_name" "$doc_file"
        echo
    done

    # Final summary
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    if [[ "$DRIFT_DETECTED" == "true" ]]; then
        log ERROR "Documentation drift detected"
        echo
        log INFO "To fix:"
        log INFO "  1. Update documentation to match actual flags"
        log INFO "  2. Or add missing flags to CLI tools"
        log INFO "  3. Run this script again to verify"
        exit 1
    else
        log SUCCESS "All documentation synced with implementation"
        exit 0
    fi
}

# Run main with all arguments
main "$@"
