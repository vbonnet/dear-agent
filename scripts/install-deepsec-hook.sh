#!/usr/bin/env bash
#
# install-deepsec-hook.sh — install a git pre-push hook that runs an
# incremental deepsec scan on the files changed in the push.
#
# Design notes:
#  - Soft-fail by default. Findings print a warning; the push proceeds.
#    CI is the hard gate. Run with --strict to make the hook exit non-zero
#    on findings (you can still bypass with `git push --no-verify`, but
#    then you've consciously chosen to).
#  - Idempotent. Re-running rewrites the deepsec block between sentinel
#    markers. Other content in the hook is left alone, so this coexists
#    with `make install-hooks` (the act-validator pre-push hook).
#  - Uninstall with --uninstall (removes only the deepsec block).
#
# Usage:
#   scripts/install-deepsec-hook.sh             # install (soft-fail)
#   scripts/install-deepsec-hook.sh --strict    # install (block on findings)
#   scripts/install-deepsec-hook.sh --uninstall # remove

set -euo pipefail

MODE="soft"
ACTION="install"

while [ $# -gt 0 ]; do
  case "$1" in
    --strict)    MODE="strict"; shift ;;
    --soft)      MODE="soft"; shift ;;
    --uninstall) ACTION="uninstall"; shift ;;
    -h|--help)   sed -n '3,19p' "$0"; exit 0 ;;
    *)           echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOKS_DIR="$(git rev-parse --git-dir)/hooks"
HOOK="$HOOKS_DIR/pre-push"
BEGIN="# >>> deepsec-incremental BEGIN (managed by scripts/install-deepsec-hook.sh)"
END="# <<< deepsec-incremental END"

mkdir -p "$HOOKS_DIR"

# Strip any existing deepsec block (idempotency + uninstall share this path).
if [ -f "$HOOK" ]; then
  tmp="$(mktemp)"
  awk -v b="$BEGIN" -v e="$END" '
    $0 == b { skip = 1; next }
    $0 == e { skip = 0; next }
    !skip   { print }
  ' "$HOOK" > "$tmp"
  mv "$tmp" "$HOOK"
fi

if [ "$ACTION" = "uninstall" ]; then
  # If the file is now empty (just shebang / `set` / comments / blanks),
  # drop it so nothing fires on push. If the user added other content,
  # leave the file alone — we only own the marked block.
  if [ -f "$HOOK" ]; then
    residue="$(grep -vE '^(#|set |\s*$)' "$HOOK" || true)"
    if [ -z "$residue" ]; then
      rm -f "$HOOK"
      echo "Removed empty $HOOK"
    else
      echo "Removed deepsec block from $HOOK (other hook content preserved)"
    fi
  fi
  exit 0
fi

# Ensure shebang + executable bit.
if [ ! -s "$HOOK" ]; then
  printf '#!/usr/bin/env bash\nset -e\n' > "$HOOK"
fi
chmod +x "$HOOK"

soft_flag=""
[ "$MODE" = "soft" ] && soft_flag=" --soft"

# Append the deepsec block. The hook receives stdin from git (lines of
# `local-ref local-sha remote-ref remote-sha`); we don't need it — we just
# diff vs the remote-tracking branch.
cat >> "$HOOK" <<HOOK_BLOCK
$BEGIN
# Runs deepsec on files changed vs origin/main. \$0 vs git's stdin: git
# pre-push pipes ref tuples; we ignore them (the diff range covers it).
if [ -x "$REPO_ROOT/scripts/deepsec-incremental.sh" ]; then
  if [ -n "\${DEEPSEC_SKIP:-}" ]; then
    echo "[pre-push] DEEPSEC_SKIP set — skipping deepsec scan" >&2
  else
    "$REPO_ROOT/scripts/deepsec-incremental.sh" --since origin/main$soft_flag || rc=\$?
    if [ "\${rc:-0}" -ne 0 ]; then exit "\$rc"; fi
  fi
fi
$END
HOOK_BLOCK

echo "Installed deepsec pre-push hook ($MODE mode) at $HOOK"
echo "Bypass for a single push: DEEPSEC_SKIP=1 git push"
echo "Uninstall: scripts/install-deepsec-hook.sh --uninstall"
