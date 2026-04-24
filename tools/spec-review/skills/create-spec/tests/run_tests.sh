#!/bin/bash
# Test runner for create-spec skill

set -e

echo "========================================"
echo "CREATE-SPEC SKILL TEST SUITE"
echo "========================================"
echo ""

# Change to tests directory
cd "$(dirname "$0")"

# Run tests
echo "Running tests..."
python test_create_spec.py

exit_code=$?

if [ $exit_code -eq 0 ]; then
    echo ""
    echo "========================================"
    echo "✓ ALL TESTS PASSED (100%)"
    echo "========================================"
else
    echo ""
    echo "========================================"
    echo "✗ TESTS FAILED"
    echo "========================================"
fi

exit $exit_code
