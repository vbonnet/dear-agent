#!/usr/bin/env bash
#
# Installer for Pre-Execution Blocker for Bash Tool
# Installs pretool-bash-blocker.py to ~/.claude/hooks/
#

set -e

HOOK_DIR="$HOME/.claude/hooks"
HOOK_FILE="$HOOK_DIR/pretool-bash-blocker.py"
SRC_FILE="${ENGRAM_ROOT:-$HOME/src/engram}/hooks/pretool-bash-blocker.py"

echo "========================================="
echo "Pre-Execution Blocker Installer"
echo "========================================="
echo ""

# Step 1: Create hooks directory if needed
echo "Step 1: Creating hooks directory..."
mkdir -p "$HOOK_DIR"
echo "✅ Directory ready: $HOOK_DIR"
echo ""

# Step 2: Verify source file exists
echo "Step 2: Verifying source file..."
if [ ! -f "$SRC_FILE" ]; then
    echo "❌ ERROR: Source file not found: $SRC_FILE"
    echo ""
    echo "Please ensure you've cloned the repository and the file exists."
    exit 1
fi
echo "✅ Source file found: $SRC_FILE"
echo ""

# Step 3: Copy to destination
echo "Step 3: Installing hook..."
cp "$SRC_FILE" "$HOOK_FILE"
echo "✅ Copied to: $HOOK_FILE"
echo ""

# Step 4: Make executable
echo "Step 4: Setting permissions..."
chmod +x "$HOOK_FILE"

if [ -x "$HOOK_FILE" ]; then
    echo "✅ Hook is executable"
else
    echo "❌ ERROR: Failed to make hook executable"
    exit 1
fi
echo ""

# Step 5: Verify installation
echo "Step 5: Verifying installation..."
if python3 -m py_compile "$HOOK_FILE"; then
    echo "✅ Python syntax valid"
else
    echo "❌ ERROR: Python syntax error in hook"
    exit 1
fi
echo ""

echo "========================================="
echo "✅ Installation Complete!"
echo "========================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Edit ~/.claude/settings.json and add:"
echo ""
echo '   "PreToolUse": ['
echo '     {'
echo '       "matcher": "Bash",'
echo '       "hooks": ['
echo '         {'
echo '           "type": "command",'
echo '           "command": "$HOME/.claude/hooks/pretool-bash-blocker.py",'
echo '           "description": "Block forbidden bash patterns, allow exceptions"'
echo '         }'
echo '       ]'
echo '     }'
echo '   ]'
echo ""
echo "2. Restart Claude Code to activate the hook"
echo ""
echo "3. Test the hook:"
echo '   - Try: git -C /tmp status (should ALLOW)'
echo '   - Try: cat file.txt (should BLOCK with helpful message)'
echo ""
echo "For troubleshooting, set DEBUG=1:"
echo '   DEBUG=1 claude-code'
echo ""
