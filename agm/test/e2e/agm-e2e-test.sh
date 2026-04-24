#!/usr/bin/env bash
# AGM E2E Test Runner
# Usage: ./agm-e2e-test.sh [--suite SUITE_NUM] [--harness HARNESS] [--verbose]

set -euo pipefail

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"

# Source libraries
source "$SCRIPT_DIR/lib/helpers.sh"
source "$SCRIPT_DIR/lib/dolt-verify.sh"
source "$SCRIPT_DIR/lib/harness-detect.sh"

# Parse arguments
SUITE_FILTER=""
HARNESS_FILTER=""
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --suite) SUITE_FILTER="$2"; shift 2 ;;
        --harness) HARNESS_FILTER="$2"; shift 2 ;;
        --verbose) VERBOSE=true; shift ;;
        --help) echo "Usage: $0 [--suite NUM] [--harness NAME] [--verbose]"; exit 0 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Header
printf "# AGM E2E Test Suite\n"
printf "# Date: %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
printf "# Socket: %s\n" "$AGM_E2E_SOCKET"
printf "#\n"

# Setup
setup_e2e_env
dolt_check_available || true
detect_all_harnesses

# Run suites
for suite in "$SCRIPT_DIR"/suites/[0-9]*.sh; do
    suite_name="$(basename "$suite")"
    suite_num="${suite_name%%[-_]*}"

    # Filter by suite number if specified
    if [[ -n "$SUITE_FILTER" ]] && [[ "$suite_num" != "$SUITE_FILTER" ]]; then
        continue
    fi

    printf "\n# === Suite: %s ===\n" "$suite_name"
    source "$suite"
done

# Summary
test_summary
