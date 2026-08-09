#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
if [[ -x "${repo_root}/build/cleanup-worktrees" ]]; then
  exec "${repo_root}/build/cleanup-worktrees" "$@"
fi
if command -v go >/dev/null 2>&1; then
  bin="${TMPDIR:-/tmp}/cleanup-worktrees-$$"
  trap 'rm -f "$bin"' EXIT
  go build -o "$bin" "${repo_root}/cmd/cleanup-worktrees"
  exec "$bin" "$@"
fi
echo "error: cleanup-worktrees requires Go or build/cleanup-worktrees" >&2
exit 127
