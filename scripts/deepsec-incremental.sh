#!/usr/bin/env bash
#
# deepsec-incremental.sh — run deepsec only on files changed in a diff.
#
# Uses `deepsec process --diff <ref>` to scan just the delta against a base
# ref (default: origin/main). This is what makes deepsec cheap to run on
# every push / PR — full repo scans are minutes-to-hours of agent time;
# incremental scans on a typical PR touch a handful of files.
#
# Usage:
#   scripts/deepsec-incremental.sh                # diff vs origin/main
#   scripts/deepsec-incremental.sh --since HEAD~1 # diff vs HEAD~1
#   scripts/deepsec-incremental.sh --staged       # files in the git index
#   scripts/deepsec-incremental.sh --working      # uncommitted + untracked
#   scripts/deepsec-incremental.sh --soft         # exit 0 even on findings
#   scripts/deepsec-incremental.sh --comment-out FILE  # write PR-shaped md
#
# Exit codes:
#   0 — no findings (or --soft was passed)
#   1 — deepsec reported at least one finding
#   2 — usage / environment error
#
# Cost: $0 when run locally — deepsec auto-detects the `claude` CLI
# subscription and uses your quota. In CI, set ANTHROPIC_API_KEY (billed).

set -euo pipefail

SINCE="origin/main"
MODE="diff"
SOFT=0
COMMENT_OUT=""
PASSTHROUGH=()

while [ $# -gt 0 ]; do
  case "$1" in
    --since)
      SINCE="${2:?--since needs a ref}"
      MODE="diff"
      shift 2
      ;;
    --staged)
      MODE="staged"
      shift
      ;;
    --working)
      MODE="working"
      shift
      ;;
    --soft)
      SOFT=1
      shift
      ;;
    --comment-out)
      COMMENT_OUT="${2:?--comment-out needs a path}"
      shift 2
      ;;
    -h|--help)
      sed -n '3,25p' "$0"
      exit 0
      ;;
    *)
      PASSTHROUGH+=("$1")
      shift
      ;;
  esac
done

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

# Find the deepsec workspace. Prefer the one checked into this repo; fall
# back to the sibling worktree the team has historically used. If neither
# exists, fall back to `npx deepsec` from a temp dir — `process --diff`
# auto-creates the project, so a stub workspace is enough for one-off runs.
DEEPSEC_DIR=""
if [ -d "$REPO_ROOT/.deepsec" ]; then
  DEEPSEC_DIR="$REPO_ROOT/.deepsec"
elif [ -d "$HOME/worktrees/dear-agent/deepsec-scan/.deepsec" ]; then
  DEEPSEC_DIR="$HOME/worktrees/dear-agent/deepsec-scan/.deepsec"
fi

# Resolve the deepsec entrypoint: prefer the workspace's own install (pinned
# in pnpm-lock.yaml), fall back to a global install. Always run from inside
# DEEPSEC_DIR if we have one — deepsec resolves data/ relative to cwd, so
# running from elsewhere strands per-project state in a junk location.
run_deepsec() {
  local cd_target="${DEEPSEC_DIR:-$REPO_ROOT}"
  if [ -n "$DEEPSEC_DIR" ] && [ -x "$DEEPSEC_DIR/node_modules/.bin/deepsec" ]; then
    (cd "$cd_target" && "$DEEPSEC_DIR/node_modules/.bin/deepsec" "$@")
  elif command -v deepsec >/dev/null 2>&1; then
    (cd "$cd_target" && deepsec "$@")
  elif command -v npx >/dev/null 2>&1; then
    (cd "$cd_target" && npx --yes deepsec "$@")
  else
    echo "deepsec-incremental: no deepsec binary found (install via 'npm i -g deepsec' or run 'pnpm install' in .deepsec/)" >&2
    exit 2
  fi
}

ARGS=(process)
case "$MODE" in
  diff)    ARGS+=(--diff "$SINCE") ;;
  staged)  ARGS+=(--diff-staged) ;;
  working) ARGS+=(--diff-working) ;;
esac

if [ -n "$COMMENT_OUT" ]; then
  ARGS+=(--comment-out "$COMMENT_OUT")
fi

# When invoking from the workspace, override --root so deepsec scans this
# checkout rather than the path baked into project.json.
if [ -n "$DEEPSEC_DIR" ]; then
  ARGS+=(--root "$REPO_ROOT")
fi

ARGS+=(${PASSTHROUGH[@]+"${PASSTHROUGH[@]}"})

set +e
run_deepsec "${ARGS[@]}"
rc=$?
set -e

if [ "$SOFT" -eq 1 ] && [ "$rc" -eq 1 ]; then
  echo "deepsec-incremental: findings present (soft mode — not failing)" >&2
  exit 0
fi

exit "$rc"
