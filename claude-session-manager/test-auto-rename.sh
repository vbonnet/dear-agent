#!/bin/bash
# Test script for AGM auto-rename feature
# Location: ~/src/ai-tools/claude-session-manager/test-auto-rename.sh

set -e

echo "========================================="
echo "AGM Auto-Rename Test Script"
echo "========================================="
echo ""

BINARY="./csm-test"
if [[ ! -f "$BINARY" ]]; then
    echo "ERROR: csm-test binary not found!"
    echo "Run: go build -o csm-test ./cmd/csm"
    exit 1
fi

echo "✅ Binary found: $BINARY"
echo ""

# Check if we're in tmux
if [[ -n "$TMUX" ]]; then
    echo "⚠️  WARNING: You are inside tmux!"
    echo "Test 1 requires running OUTSIDE tmux."
    echo "Exit tmux first, then run this script."
    exit 1
fi

echo "========================================="
echo "Test 1: Outside tmux - Create New Session"
echo "========================================="
echo ""
echo "This will:"
echo "  1. Create tmux session 'csm-test-1'"
echo "  2. Start Claude"
echo "  3. Send /rename csm-test-1 command"
echo ""
echo "MANUAL VERIFICATION NEEDED:"
echo "  - Check Claude UI shows session name: 'csm-test-1'"
echo "  - Check /rename command is visible in chat"
echo ""
read -p "Press ENTER to run Test 1, or Ctrl-C to cancel..."

$BINARY new csm-test-1

echo ""
echo "✅ Test 1 command executed"
echo ""
echo "Did you see:"
echo "  1. Claude start successfully? (y/n)"
read -r answer1
echo "  2. /rename csm-test-1 command in chat? (y/n)"
read -r answer2
echo "  3. Claude session renamed to 'csm-test-1'? (y/n)"
read -r answer3

if [[ "$answer1" == "y" && "$answer2" == "y" && "$answer3" == "y" ]]; then
    echo "✅ Test 1: PASS"
else
    echo "❌ Test 1: FAIL"
    echo "   answer1=$answer1, answer2=$answer2, answer3=$answer3"
    exit 1
fi

echo ""
echo "========================================="
echo "Test 2: Resume - No Auto-Rename"
echo "========================================="
echo ""
echo "Exit Claude (if still running), then press ENTER..."
read

echo "This will resume 'csm-test-1' - should NOT send /rename again"
echo ""
read -p "Press ENTER to run Test 2..."

$BINARY resume csm-test-1

echo ""
echo "✅ Test 2 command executed"
echo ""
echo "Did you see:"
echo "  1. Session resumed successfully? (y/n)"
read -r answer4
echo "  2. NO second /rename command? (y/n)"
read -r answer5

if [[ "$answer4" == "y" && "$answer5" == "y" ]]; then
    echo "✅ Test 2: PASS"
else
    echo "❌ Test 2: FAIL"
    echo "   answer4=$answer4, answer5=$answer5"
    exit 1
fi

echo ""
echo "========================================="
echo "Test 3: Inside tmux"
echo "========================================="
echo ""
echo "This test requires manual execution:"
echo ""
echo "  1. Exit Claude if running"
echo "  2. Run: tmux new -s csm-test-2"
echo "  3. Inside tmux, run: $BINARY new"
echo "  4. Verify Claude session renamed to 'csm-test-2'"
echo ""
echo "Did you complete Test 3 successfully? (y/n)"
read -r answer6

if [[ "$answer6" == "y" ]]; then
    echo "✅ Test 3: PASS"
else
    echo "⚠️  Test 3: Not completed"
fi

echo ""
echo "========================================="
echo "TEST SUMMARY"
echo "========================================="
echo "Test 1 (outside tmux): ${answer1}${answer2}${answer3}"
echo "Test 2 (resume): ${answer4}${answer5}"
echo "Test 3 (inside tmux): ${answer6}"
echo ""

if [[ "$answer1" == "y" && "$answer2" == "y" && "$answer3" == "y" && \
      "$answer4" == "y" && "$answer5" == "y" ]]; then
    echo "✅ ALL CRITICAL TESTS PASSED!"
    echo ""
    echo "Next steps:"
    echo "  1. Clean up: rm csm-test"
    echo "  2. Commit: git add cmd/csm/new.go && git commit -m '...'"
    echo "  3. Deploy: make build && cp csm ~/.local/bin/csm"
    exit 0
else
    echo "❌ SOME TESTS FAILED - DO NOT COMMIT!"
    exit 1
fi
