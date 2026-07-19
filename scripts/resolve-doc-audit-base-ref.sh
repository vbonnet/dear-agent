#!/usr/bin/env bash
# Resolve the historical commit used by the living-document shrink-only ratchet.
set -euo pipefail

repo_root="${REPO_ROOT:-$(git rev-parse --show-toplevel)}"
cd "$repo_root"

event="${GITHUB_EVENT_NAME:-}"
candidate="${1:-}"
case "$event" in
  pull_request)
    candidate="${BASE_SHA:?BASE_SHA is required for pull_request events}"
    ;;
  push)
    before="${BEFORE_SHA:?BEFORE_SHA is required for push events}"
    current="${CURRENT_SHA:?CURRENT_SHA is required for push events}"
    if [[ ! "$before" =~ ^0+$ ]]; then
      candidate="$before"
    elif git rev-parse --verify "${current}^" >/dev/null 2>&1; then
      candidate="${current}^"
    else
      default_branch="${DEFAULT_BRANCH:?DEFAULT_BRANCH is required for root push events}"
      candidate="$(git merge-base "$current" "origin/$default_branch")"
      [[ "$candidate" != "$current" ]] || candidate=""
    fi
    ;;
  "")
    if [[ -z "$candidate" ]] && command -v gh >/dev/null 2>&1; then
      candidate="$(gh pr view --json baseRefOid --jq .baseRefOid 2>/dev/null || true)"
    fi
    [[ -n "$candidate" ]] || candidate="origin/main"
    ;;
  *)
    candidate=""
    ;;
esac

[[ -n "$candidate" ]] || exit 0
[[ "$candidate" != -* ]] || { echo "doc-audit-base-ref: invalid comparison ref '$candidate'" >&2; exit 2; }
git rev-parse --verify "${candidate}^{commit}" >/dev/null
if [[ -z "$event" ]]; then
  git merge-base HEAD "$candidate"
else
  git rev-parse "${candidate}^{commit}"
fi
