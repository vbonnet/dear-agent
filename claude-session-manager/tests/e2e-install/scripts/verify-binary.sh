#!/usr/bin/env bash
set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helpers
# shellcheck source=./test-helpers.sh
. "$SCRIPT_DIR/test-helpers.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Binary Verification Tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Test 1: Binary in PATH
log_info "Test: csm command exists"
assert_command_exists "csm"

# Test 2: Version command works
log_info "Test: csm version works"
VERSION_OUTPUT=$(csm version)
assert_exit_code 0
assert_output_contains "$VERSION_OUTPUT" "version"

# Test 3: Binary location
log_info "Test: Binary installed to expected location"
BINARY_PATH=$(command -v csm)
log_info "Binary location: $BINARY_PATH"

# Expected: $HOME/go/bin/csm or similar
if [[ "$BINARY_PATH" == *"go/bin/csm"* ]]; then
    log_success "Binary in expected location (go/bin)"
else
    log_error "Binary not in expected location (found: $BINARY_PATH)"
    exit 1
fi

echo ""
log_success "All binary verification tests passed"

exit 0
