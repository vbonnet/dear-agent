#!/bin/bash
# Install AGM slash commands to global Claude commands directory

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMANDS_SRC="$SCRIPT_DIR/agm-plugin/commands"
COMMANDS_DST="$HOME/.claude/commands"

echo "Installing AGM slash commands..."

for cmd in "$COMMANDS_SRC"/*; do
    if [ -f "$cmd" ]; then
        cmd_name=$(basename "$cmd")
        echo "  Installing /$cmd_name"
        cp "$cmd" "$COMMANDS_DST/$cmd_name"
        chmod +x "$COMMANDS_DST/$cmd_name"
    fi
done

echo "✓ AGM commands installed to $COMMANDS_DST"
echo ""
echo "Available commands:"
ls -1 "$COMMANDS_DST" | grep -E "^agm-" | sed 's/^/  \//'
