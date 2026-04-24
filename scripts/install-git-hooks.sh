#!/bin/bash
#
# Install Git Hooks - Fork Bomb Prevention
#
# This script installs git hooks that automatically remove dangerous .old hook files
# that can cause fork bombs through infinite recursive execution.
#
# Usage: ./scripts/install-git-hooks.sh
#

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOKS_DIR="$(git rev-parse --git-dir)/hooks"

echo "Installing git hooks for fork bomb prevention..."
echo "Repo root: $REPO_ROOT"
echo "Hooks dir: $HOOKS_DIR"
echo ""

# Ensure hooks directory exists
mkdir -p "$HOOKS_DIR"

# Install post-checkout hook
cat > "$HOOKS_DIR/post-checkout" <<'HOOK_EOF'
#!/bin/bash
#
# Post-checkout Hook - Automatic .old Hook Cleanup
#
# This hook automatically removes dangerous .old hook files that can cause
# fork bombs through infinite recursive execution.
#
# Runs after: git checkout, git clone, git worktree add
#

HOOKS_DIR="$(git rev-parse --git-dir)/hooks"

# Find and remove any .old hook files
if [ -d "$HOOKS_DIR" ]; then
    OLD_HOOKS=$(find "$HOOKS_DIR" -maxdepth 1 -name '*.old' -type f 2>/dev/null)
    
    if [ -n "$OLD_HOOKS" ]; then
        echo "[post-checkout] Removing dangerous .old hook files:"
        echo "$OLD_HOOKS" | while read -r hook; do
            echo "  Removed: $(basename "$hook")"
            rm -f "$hook"
        done
    fi
fi

exit 0
HOOK_EOF

chmod +x "$HOOKS_DIR/post-checkout"

echo "✓ Installed post-checkout hook"
echo ""
echo "Done! The hook will automatically remove .old hook files after git operations."
