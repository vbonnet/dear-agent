#!/usr/bin/env bash
# Test script for CI gate system
#
# This script tests the CI gate policy system without requiring a full merge.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "🧪 Testing CI Gate System"
echo "=========================="

# Test 1: Validate policy file
echo ""
echo "Test 1: Validating policy configuration..."
if [ -f "$REPO_ROOT/.ci-policy.yaml" ]; then
    echo "✅ Policy file exists: .ci-policy.yaml"

    # Try to load and validate (requires hook binary)
    if [ -f "$REPO_ROOT/pre-merge-commit" ]; then
        if "$REPO_ROOT/pre-merge-commit" --dry-run --verbose; then
            echo "✅ Policy validation passed"
        else
            echo "❌ Policy validation failed"
            exit 1
        fi
    else
        echo "ℹ️  Hook binary not found, skipping validation"
    fi
else
    echo "⚠️  No policy file found (using defaults)"
fi

# Test 2: Run unit tests
echo ""
echo "Test 2: Running policy unit tests..."
cd "$REPO_ROOT"
if go test ./internal/ci/... -v -short; then
    echo "✅ Unit tests passed"
else
    echo "❌ Unit tests failed"
    exit 1
fi

# Test 3: Build hook binary
echo ""
echo "Test 3: Building pre-merge-commit hook..."
if go build -o "$REPO_ROOT/pre-merge-commit" ./cmd/agm-hooks/pre-merge-commit; then
    echo "✅ Hook binary built successfully"
else
    echo "❌ Failed to build hook binary"
    exit 1
fi

# Test 4: Test help output
echo ""
echo "Test 4: Testing help output..."
if "$REPO_ROOT/pre-merge-commit" --help > /dev/null; then
    echo "✅ Help command works"
else
    echo "❌ Help command failed"
    exit 1
fi

# Test 5: Test dry-run mode
echo ""
echo "Test 5: Testing dry-run mode..."
if "$REPO_ROOT/pre-merge-commit" --dry-run --verbose; then
    echo "✅ Dry-run mode works"
else
    echo "❌ Dry-run mode failed"
    exit 1
fi

echo ""
echo "=========================="
echo "✅ All tests passed!"
echo ""
echo "To use the CI gate system:"
echo "  1. Copy hook to .git/hooks:"
echo "     cp pre-merge-commit .git/hooks/"
echo "  2. Customize .ci-policy.yaml"
echo "  3. Perform a merge to test"
echo ""
