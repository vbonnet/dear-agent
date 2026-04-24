#!/usr/bin/env bash
#
# Verify Gemini Hook Tests Compile
#
# This script verifies that the Gemini hook tests compile without errors.
#

set -euo pipefail

cd "$(dirname "$0")/../../.."

echo "Verifying Gemini hook tests compile..."

# Check if test compiles
if go test -tags=integration -c -o /dev/null ./test/integration/lifecycle 2>&1; then
    echo "✓ Tests compile successfully"
    exit 0
else
    echo "✗ Test compilation failed"
    exit 1
fi
