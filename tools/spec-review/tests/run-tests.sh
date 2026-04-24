#!/usr/bin/env bash
# Test runner for spec-review-marketplace
# Runs all test suites

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=========================================="
echo "Spec Review Marketplace Tests"
echo "=========================================="
echo ""

# Run CLI abstraction tests
echo "Running CLI abstraction tests..."
python3 "${SCRIPT_DIR}/test_cli_abstraction.py"

echo ""

# Run discovery tests
echo "Running discovery tests..."
pytest "${SCRIPT_DIR}/test_discovery.py" -v

echo ""
echo "=========================================="
echo "All tests completed!"
echo "=========================================="
