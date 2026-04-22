#!/bin/bash
# BDD test runner for AI-Tools Phase 1
# Verifies session-lifecycle and agent-selection features

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "===================="
echo "BDD Test Verification"
echo "===================="
echo ""

# Check if godog is available
if ! go list -m github.com/cucumber/godog >/dev/null 2>&1; then
    echo "ERROR: godog not found in go.mod"
    exit 1
fi
echo "✓ godog dependency verified"

# Verify feature files exist
if [ ! -f "features/session_lifecycle.feature" ]; then
    echo "ERROR: features/session_lifecycle.feature not found"
    exit 1
fi
echo "✓ session_lifecycle.feature exists"

if [ ! -f "features/agent_selection.feature" ]; then
    echo "ERROR: features/agent_selection.feature not found"
    exit 1
fi
echo "✓ agent_selection.feature exists"

# Verify step definition files exist
if [ ! -f "steps/session_steps.go" ]; then
    echo "ERROR: steps/session_steps.go not found"
    exit 1
fi
echo "✓ session_steps.go exists"

if [ ! -f "steps/setup_steps.go" ]; then
    echo "ERROR: steps/setup_steps.go not found"
    exit 1
fi
echo "✓ setup_steps.go exists"

if [ ! -f "steps/conversation_steps.go" ]; then
    echo "ERROR: steps/conversation_steps.go not found"
    exit 1
fi
echo "✓ conversation_steps.go exists"

# Verify mock adapters exist
if [ ! -f "internal/adapters/mock/adapter.go" ]; then
    echo "ERROR: internal/adapters/mock/adapter.go not found"
    exit 1
fi
echo "✓ mock adapter interfaces exist"

# Verify test environment exists
if [ ! -f "internal/testenv/environment.go" ]; then
    echo "ERROR: internal/testenv/environment.go not found"
    exit 1
fi
echo "✓ test environment exists"

echo ""
echo "===================="
echo "Running BDD Tests"
echo "===================="
echo ""

# Run tests with verbose output
go test -v -timeout 5m

echo ""
echo "===================="
echo "✓ All BDD Tests Passed"
echo "===================="
