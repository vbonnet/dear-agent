#!/bin/bash
# Integration test runner for Wayfinder V2

set -e

echo "=================================================="
echo "Wayfinder V2 Integration Test Suite"
echo "=================================================="
echo ""

# Get the cortex root directory
CORTEX_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$CORTEX_ROOT"

# Build wayfinder-session binary
echo "📦 Building wayfinder-session..."
go build -o /tmp/wayfinder-session ./cmd/wayfinder-session
if [ $? -ne 0 ]; then
    echo "❌ Failed to build wayfinder-session"
    exit 1
fi
echo "✅ wayfinder-session built successfully"
echo ""

# Add to PATH
export PATH="/tmp:$PATH"

# Verify binary is accessible
if ! command -v wayfinder-session &> /dev/null; then
    echo "❌ wayfinder-session not found in PATH"
    exit 1
fi

echo "🔍 wayfinder-session version:"
wayfinder-session --version || echo "  (version command not implemented)"
echo ""

# Run tests
echo "🧪 Running integration tests..."
echo ""

if [ "$1" == "--short" ]; then
    echo "Running in short mode (skipping integration tests)"
    go test -short -v ./test/integration/...
elif [ -n "$1" ]; then
    echo "Running specific test: $1"
    go test -v ./test/integration/... -run "$1"
else
    echo "Running full test suite"
    go test -v ./test/integration/...
fi

TEST_EXIT_CODE=$?

echo ""
echo "=================================================="
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ All tests passed!"
else
    echo "❌ Some tests failed (exit code: $TEST_EXIT_CODE)"
fi
echo "=================================================="

exit $TEST_EXIT_CODE
