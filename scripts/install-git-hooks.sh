#!/bin/bash
#
# Install Git Hooks - Fork Bomb Prevention
#
# Installs a post-checkout hook that removes dangerous `.old` hook files, which
# can cause fork bombs through infinite recursive execution.
#
# core.hooksPath-aware. git consults a single hooks dir, and core.hooksPath
# (global OR repo-local) overrides the repo's own .git/hooks. On the maintainer
# host core.hooksPath is set — globally to ~/.config/git/hooks (chezmoi-managed)
# and, in some repos, locally to .beads/hooks (beads). In either case a
# repo-local .git/hooks/post-checkout is SILENTLY BYPASSED by git. Rather than
# write into the void (or into a centrally-managed / golden checkout dir), this
# script only installs when the effective hooks dir IS this repo's own
# .git/hooks; otherwise it prints how to wire the cleanup in and exits cleanly.
# The resolution mirrors cmd/install-post-merge-hook (Go).
#
# Usage: ./scripts/install-git-hooks.sh
#

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"

# Resolve a possibly-relative hooks path to an absolute one, mirroring git: a
# leading ~ expands to $HOME, a relative path resolves against the repo root.
abspath() {
    local p="$1"
    # Strip a literal leading "~/" and prepend $HOME (string op, not tilde
    # expansion — git stores core.hooksPath verbatim, e.g. "~/.config/...").
    if [ "${p#\~/}" != "$p" ]; then
        p="$HOME/${p#\~/}"
    fi
    case "$p" in
        /*) printf '%s\n' "$p" ;;
        *)  printf '%s\n' "$REPO_ROOT/$p" ;;
    esac
}

# The repo's OWN hooks dir — what git would use if core.hooksPath were unset.
# NOTE: `git rev-parse --git-path hooks` HONORS core.hooksPath, so it cannot be
# used here; --git-common-dir gives the real .git dir (shared across worktrees).
OWN_HOOKS="$(abspath "$(git rev-parse --git-common-dir)/hooks")"

# The hooks dir git ACTUALLY consults (core.hooksPath wins when set).
HP="$(git config --get core.hooksPath || true)"
if [ -n "$HP" ]; then
    EFFECTIVE="$(abspath "$HP")"
else
    EFFECTIVE="$OWN_HOOKS"
fi

if [ "$EFFECTIVE" != "$OWN_HOOKS" ]; then
    cat <<EOF
core.hooksPath redirects git's hooks dir away from this repo:
  effective hooks dir : $EFFECTIVE
  repo-local .git/hooks: $OWN_HOOKS

A repo-local post-checkout hook will NOT run on this host — git consults the
effective dir above instead. That dir is centrally managed (chezmoi global
~/.config/git/hooks, or a beads .beads/hooks), so this script will not write
into it. To get the fork-bomb .old-cleanup here, add a post-checkout hook to
whatever owns the effective dir (e.g. the chezmoi source dot_config/git/hooks/,
which goes through the chezmoi review workflow).

Nothing was installed. On a centrally-managed host this is expected, not an error.
EOF
    exit 0
fi

DEST="$EFFECTIVE/post-checkout"

echo "Installing post-checkout hook for fork bomb prevention..."
echo "Repo root: $REPO_ROOT"
echo "Hooks dir: $EFFECTIVE"
echo ""

# Never silently clobber a foreign post-checkout hook. The marker below makes
# re-running this script idempotent.
if [ -e "$DEST" ] && ! grep -q 'dear-agent: .old hook cleanup' "$DEST" 2>/dev/null; then
    echo "Error: a post-checkout hook already exists at $DEST" >&2
    echo "Merge the .old-cleanup into it manually, or remove it first." >&2
    exit 1
fi

mkdir -p "$EFFECTIVE"

cat > "$DEST" <<'HOOK_EOF'
#!/bin/bash
# dear-agent: .old hook cleanup
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

chmod +x "$DEST"

echo "✓ Installed post-checkout hook -> $DEST"
echo ""
echo "Done! The hook will automatically remove .old hook files after git operations."
