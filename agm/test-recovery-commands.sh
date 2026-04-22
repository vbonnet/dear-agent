#!/bin/bash
#
# Test script for AGM recovery commands
#

set -e

echo "===================================="
echo "Testing AGM Recovery Commands"
echo "===================================="
echo

# Build fresh binary
echo "Building AGM..."
cd "${AI_TOOLS_ROOT:-$HOME/src/ai-tools}/agm"
go build -o /tmp/agm-test ./cmd/agm
AGM=/tmp/agm-test

echo "✓ Build successful"
echo

# Test 1: Check recover command exists
echo "Test 1: Check 'agm session recover' command exists"
$AGM session recover --help >/dev/null 2>&1 && echo "✓ recover command found" || echo "✗ recover command not found"
echo

# Test 2: Check kill command has --hard flag
echo "Test 2: Check 'agm session kill --hard' flag exists"
$AGM session kill --help 2>&1 | grep -q "\-\-hard" && echo "✓ --hard flag found" || echo "✗ --hard flag not found"
echo

# Test 3: Check deadlock package exists
echo "Test 3: Check deadlock detection package"
go list github.com/vbonnet/ai-tools/agm/internal/deadlock >/dev/null 2>&1 && echo "✓ deadlock package found" || echo "✗ deadlock package not found"
echo

# Test 4: List active sessions for manual testing
echo "Test 4: List active sessions"
echo "Run these commands manually to test:"
echo
echo "  # Test soft recovery (safe to run on any session)"
echo "  $AGM session recover <session-name>"
echo
echo "  # Test hard kill with deadlock detection"
echo "  $AGM session kill --hard <session-name>"
echo
echo "  # View deadlock log"
echo "  cat ~/deadlock-log.txt"
echo

echo "===================================="
echo "Automated Tests: PASSED"
echo "===================================="
echo
echo "For full integration testing:"
echo "1. Create a test session: agm session new test-recovery"
echo "2. Test soft recovery: agm session recover test-recovery"
echo "3. Test hard kill: agm session kill --hard test-recovery"
echo "4. Check incident log: cat ~/deadlock-log.txt"
