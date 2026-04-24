#!/bin/bash
# Test script to verify the AGM SessionStart hook works correctly
# Usage: ./test-hook.sh

set -e

echo "🧪 Testing AGM SessionStart Hook"
echo "================================"
echo

# Test 1: Hook script exists and is executable
echo "Test 1: Hook script exists and is executable"
if [ -x "$(dirname "$0")/session-start-agm.sh" ]; then
    echo "✓ Hook script found and executable"
else
    echo "✗ Hook script not found or not executable"
    echo "  Run: chmod +x $(dirname "$0")/session-start-agm.sh"
    exit 1
fi
echo

# Test 2: Hook script runs without errors
echo "Test 2: Hook script runs without errors (no AGM_SESSION_NAME)"
if "$(dirname "$0")/session-start-agm.sh" 2>&1; then
    echo "✓ Hook script runs successfully (skipped, no AGM_SESSION_NAME)"
else
    echo "✗ Hook script failed"
    exit 1
fi
echo

# Test 3: Hook script creates ready-file when AGM_SESSION_NAME is set
echo "Test 3: Hook creates ready-file when AGM_SESSION_NAME is set"
TEST_SESSION="hook-test-$$"
export AGM_SESSION_NAME="$TEST_SESSION"

# Cleanup any existing ready-file
rm -f ~/.agm/claude-ready-$TEST_SESSION

# Run hook
if "$(dirname "$0")/session-start-agm.sh" 2>&1 | grep -q "Claude ready signal created"; then
    echo "✓ Hook logged success message"
else
    echo "⚠ Hook did not log success message (might still work)"
fi

# Check ready-file was created
if [ -f ~/.agm/claude-ready-$TEST_SESSION ]; then
    echo "✓ Ready-file created: ~/.agm/claude-ready-$TEST_SESSION"
else
    echo "✗ Ready-file NOT created"
    exit 1
fi

# Cleanup
rm -f ~/.agm/claude-ready-$TEST_SESSION
unset AGM_SESSION_NAME
echo

# Test 4: Check if hook is configured in Claude config
echo "Test 4: Check Claude config for hook configuration"
if [ -f ~/.config/claude/config.yaml ]; then
    if grep -q "session-start-agm.sh" ~/.config/claude/config.yaml; then
        echo "✓ Hook is configured in ~/.config/claude/config.yaml"
    else
        echo "⚠ Hook NOT configured in ~/.config/claude/config.yaml"
        echo "  Add this to your config:"
        echo ""
        echo "  hooks:"
        echo "    SessionStart:"
        echo "      - name: agm-ready-signal"
        echo "        command: $HOME/.config/claude/hooks/session-start-agm.sh"
        echo ""
    fi
else
    echo "⚠ Claude config not found at ~/.config/claude/config.yaml"
    echo "  You need to create it with:"
    echo ""
    echo "  mkdir -p ~/.config/claude"
    echo "  cat > ~/.config/claude/config.yaml <<'EOF'"
    echo "  hooks:"
    echo "    SessionStart:"
    echo "      - name: agm-ready-signal"
    echo "        command: $HOME/.config/claude/hooks/session-start-agm.sh"
    echo "  EOF"
    echo ""
fi
echo

# Test 5: Check if hook is installed in Claude hooks directory
echo "Test 5: Check if hook is installed in ~/.config/claude/hooks/"
if [ -f ~/.config/claude/hooks/session-start-agm.sh ]; then
    if [ -x ~/.config/claude/hooks/session-start-agm.sh ]; then
        echo "✓ Hook installed and executable at ~/.config/claude/hooks/session-start-agm.sh"
    else
        echo "⚠ Hook installed but NOT executable"
        echo "  Run: chmod +x ~/.config/claude/hooks/session-start-agm.sh"
    fi
else
    echo "⚠ Hook NOT installed at ~/.config/claude/hooks/session-start-agm.sh"
    echo "  Run: cp $(dirname "$0")/session-start-agm.sh ~/.config/claude/hooks/"
    echo "  Then: chmod +x ~/.config/claude/hooks/session-start-agm.sh"
fi
echo

echo "================================"
echo "✅ All hook tests passed!"
echo ""
echo "Next steps:"
echo "  1. Ensure hook is configured in ~/.config/claude/config.yaml (see Test 4 above)"
echo "  2. Ensure hook is installed in ~/.config/claude/hooks/ (see Test 5 above)"
echo "  3. Test with: agm new test-hook --debug"
echo "  4. Verify ready-file appears: ls ~/.agm/claude-ready-test-hook"
