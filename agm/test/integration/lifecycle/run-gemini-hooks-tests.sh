#!/usr/bin/env bash
#
# Run Gemini CLI Hook Integration Tests
#
# This script runs integration tests for Gemini CLI hook execution via AGM.
#
# Prerequisites:
#   - Gemini CLI installed (gemini command available)
#   - Go installed
#   - tmux installed
#   - jq installed (optional, but recommended for JSON parsing in hooks)
#
# Usage:
#   ./run-gemini-hooks-tests.sh
#   ./run-gemini-hooks-tests.sh -v     # Verbose output
#   ./run-gemini-hooks-tests.sh -short # Skip integration tests
#

set -euo pipefail

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date -u +%Y-%m-%dT%H:%M:%SZ)]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Check prerequisites
check_prerequisites() {
    local missing=0

    if ! command -v go &>/dev/null; then
        error "Go is not installed"
        missing=1
    fi

    if ! command -v tmux &>/dev/null; then
        error "tmux is not installed"
        missing=1
    fi

    if ! command -v gemini &>/dev/null; then
        warn "Gemini CLI is not installed (tests will be skipped)"
        warn "Install from: https://github.com/google/gemini-cli"
    fi

    if ! command -v jq &>/dev/null; then
        warn "jq is not installed (recommended for hook JSON parsing)"
        warn "Install: brew install jq / apt-get install jq"
    fi

    if [[ $missing -eq 1 ]]; then
        error "Missing required dependencies"
        exit 1
    fi
}

# Run tests
run_tests() {
    log "Running Gemini CLI hook integration tests..."

    # Change to agm directory
    cd "$(dirname "$0")/../../.."

    # Build test flags
    local test_flags="-tags=integration -v"

    if [[ "${1:-}" == "-short" ]]; then
        test_flags="$test_flags -short"
    fi

    # Run tests
    log "Test command: go test $test_flags ./test/integration/lifecycle -run TestGeminiHooks"

    if go test $test_flags ./test/integration/lifecycle -run TestGeminiHooks; then
        log "${GREEN}✓ All Gemini hook tests passed${NC}"
        return 0
    else
        error "${RED}✗ Some tests failed${NC}"
        return 1
    fi
}

# Main
main() {
    log "Gemini CLI Hook Integration Test Runner"
    log "========================================"

    check_prerequisites

    if run_tests "$@"; then
        log ""
        log "Test suite completed successfully"
        exit 0
    else
        error ""
        error "Test suite failed"
        exit 1
    fi
}

main "$@"
