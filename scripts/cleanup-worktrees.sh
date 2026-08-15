#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
export GIT_TERMINAL_PROMPT=0

if [[ -x "$ROOT/bin/cleanup-worktrees" ]]; then
  exec "$ROOT/bin/cleanup-worktrees" "$@"
fi

TMP="${TMPDIR:-/tmp}/cleanup-worktrees.$$"
trap 'rm -f "$TMP"' EXIT
go build -o "$TMP" "$ROOT/cmd/cleanup-worktrees"
exec "$TMP" "$@"
