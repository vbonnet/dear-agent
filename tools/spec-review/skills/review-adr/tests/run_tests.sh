#!/usr/bin/env bash
# Test runner for review-adr skill
set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PLUGIN_ROOT="$(cd "${SKILL_DIR}/../.." && pwd)"

echo "=== review-adr Skill Test Suite ==="
echo "Skill: ${SKILL_DIR}"
echo "Plugin: ${PLUGIN_ROOT}"
echo ""

# Check Python version
echo "Checking Python version..."
python3 --version

# Check dependencies
echo ""
echo "Checking dependencies..."
if ! python3 -c "import pytest" 2>/dev/null; then
    echo "Installing pytest..."
    pip install pytest
fi

# Run tests
echo ""
echo "Running tests..."
cd "${SKILL_DIR}"
python3 -m pytest tests/test_review_adr.py -v --tb=short

# Test CLI adapters
echo ""
echo "=== Testing CLI Adapters ==="

# Create test ADR
TEST_ADR="${SCRIPT_DIR}/test_adr.md"
cat > "${TEST_ADR}" <<'EOF'
# ADR-001: Test ADR

## Status
Accepted

## Context
This is a test ADR for validation.

## Decision
Use testing framework.

## Consequences
- Positive: Better quality
- Negative: More time
EOF

echo ""
echo "Testing Claude Code adapter..."
python3 cli-adapters/claude-code.py "${TEST_ADR}" || true

echo ""
echo "Testing Gemini adapter..."
python3 cli-adapters/gemini.py "${TEST_ADR}" || true

echo ""
echo "Testing OpenCode adapter..."
python3 cli-adapters/opencode.py "${TEST_ADR}" || true

echo ""
echo "Testing Codex adapter..."
python3 cli-adapters/codex.py "${TEST_ADR}" || true

# Cleanup
rm -f "${TEST_ADR}"

echo ""
echo "=== All Tests Complete ==="
