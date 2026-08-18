#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMANDS_SRC="$SCRIPT_DIR/agm-plugin/commands"
COMMANDS_DST="$HOME/.claude/commands"
echo "Installing AGM slash commands..."
mkdir -p "$COMMANDS_DST"
rm -f "$COMMANDS_DST"/{SPEC.md,allowed_tools_test.go,agm-exit-bow-gate_test.sh,agm-exit_test.sh,audit-completion_test.sh}
for cmd in "$COMMANDS_SRC"/agm-*.md "$COMMANDS_SRC"/audit-completion.md "$COMMANDS_SRC"/wiki-*.md; do
    [[ -f "$cmd" ]] || continue
    cmd_name=$(basename "$cmd")
    echo "  Installing /$cmd_name"
    cp "$cmd" "$COMMANDS_DST/$cmd_name"
done
echo "✓ AGM commands installed to $COMMANDS_DST"
