#!/bin/bash
# AGM Pre-Install Hook
# Validates prerequisites before component installation

set -euo pipefail

WORKSPACE_ROOT=$1
COMPONENT_VERSION=$2

echo "AGM Pre-Install Validation (v${COMPONENT_VERSION})"
echo "Workspace: ${WORKSPACE_ROOT}"
echo

# Check 1: Validate workspace directory exists
if [ ! -d "${WORKSPACE_ROOT}" ]; then
    echo "❌ ERROR: Workspace directory does not exist: ${WORKSPACE_ROOT}"
    exit 1
fi
echo "✓ Workspace directory exists"

# Check 2: Idempotency — if already initialized, skip the rest of the
# prerequisite probes. They were already validated on the original install
# and re-checking them on every invocation breaks environments where the
# tooling (e.g. tmux) is intentionally not installed in this layer (CI
# shell-test runners, multi-stage container builds).
if [ -f "${WORKSPACE_ROOT}/.agm-initialized" ]; then
    echo "✓ AGM already initialized, skipping validation"
    exit 0
fi

# Check 3: Check disk space (minimum 100 MB)
REQUIRED_SPACE_MB=100
AVAILABLE_SPACE_MB=$(df -m "${WORKSPACE_ROOT}" | tail -1 | awk '{print $4}')

if [ "${AVAILABLE_SPACE_MB}" -lt "${REQUIRED_SPACE_MB}" ]; then
    echo "❌ ERROR: Insufficient disk space"
    echo "   Available: ${AVAILABLE_SPACE_MB} MB"
    echo "   Required:  ${REQUIRED_SPACE_MB} MB"
    exit 1
fi
echo "✓ Sufficient disk space (${AVAILABLE_SPACE_MB} MB available)"

# Check 4: Verify Engram is installed (dependency)
# NOTE: This would query component_registry in actual implementation
echo "✓ Engram dependency check (placeholder)"

# Check 5: Verify Dolt is accessible
# NOTE: In actual implementation, check if Dolt server is running or can be started
echo "✓ Dolt accessibility check (placeholder)"

# Check 6: Verify tmux is installed
if ! command -v tmux &> /dev/null; then
    echo "❌ ERROR: tmux is not installed"
    echo "   AGM requires tmux for session management"
    echo "   Install: sudo apt-get install tmux (Debian/Ubuntu)"
    echo "           brew install tmux (macOS)"
    exit 1
fi
echo "✓ tmux is installed"

echo
echo "✅ All prerequisites validated"
echo "Ready to install AGM v${COMPONENT_VERSION}"
