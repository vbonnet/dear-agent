#!/usr/bin/env bash
# Thin shim for cmd/cleanup-worktrees. Prefers a prebuilt binary and
# otherwise builds one from the module root, so an absolute invocation from
# any working directory still resolves this repository's go.mod.
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
export GIT_TERMINAL_PROMPT=0

if [[ -x "$ROOT/bin/cleanup-worktrees" ]]; then
  exec "$ROOT/bin/cleanup-worktrees" "$@"
fi

# Not exec: the EXIT trap must run so the temporary build is not leaked.
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
(cd -- "$ROOT" && go build -o "$TMP/cleanup-worktrees" ./cmd/cleanup-worktrees)
"$TMP/cleanup-worktrees" "$@"
