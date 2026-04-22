#!/bin/bash
# Uninstall AGM slash commands from global Claude commands directory

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMANDS_SRC="$SCRIPT_DIR/agm-plugin/commands"
COMMANDS_DST="$HOME/.claude/commands"

echo "Uninstalling AGM slash commands..."

# Unregister from Corpus Callosum first (optional - graceful degradation)
echo "Unregistering from Corpus Callosum..."
if [ -f "$SCRIPT_DIR/scripts/unregister-corpus-callosum.sh" ]; then
    bash "$SCRIPT_DIR/scripts/unregister-corpus-callosum.sh" || true
else
    echo "INFO: Corpus Callosum unregistration script not found - skipping"
fi

echo ""
echo "Removing command files..."

for cmd in "$COMMANDS_SRC"/*; do
    if [ -f "$cmd" ]; then
        cmd_name=$(basename "$cmd")
        cmd_path="$COMMANDS_DST/$cmd_name"
        if [ -f "$cmd_path" ]; then
            echo "  Removing /$cmd_name"
            rm "$cmd_path"
        fi
    fi
done

echo "✓ AGM commands uninstalled"
