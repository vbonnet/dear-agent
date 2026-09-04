#!/usr/bin/env bash
# isolated-preflight.sh: run preflight in an isolated temporary environment.
#
# Provisions an isolated HOME, GOCACHE, and TMPDIR under a single dedicated
# parent directory: ${XDG_CACHE_HOME:-$HOME/.cache}/dear-agent/preflight-tmp.
# Registers an exit trap ensuring the scratch directory is removed on exit
# or signal interruption.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

HOST_HOME="${HOME:-}"
CACHE_ROOT="${XDG_CACHE_HOME:-$HOST_HOME/.cache}"
PREFLIGHT_PARENT="${PREFLIGHT_TMP_DIR:-$CACHE_ROOT/dear-agent/preflight-tmp}"

mkdir -p "$PREFLIGHT_PARENT"
chmod 700 "$PREFLIGHT_PARENT"

RUN_DIR="$(mktemp -d "$PREFLIGHT_PARENT/run.XXXXXX")"

cleanup() {
    chmod -R u+w "$RUN_DIR" 2>/dev/null || true
    rm -rf "$RUN_DIR" 2>/dev/null || true
}
trap cleanup EXIT INT TERM HUP

# Establish isolated environment roots
ISOLATED_HOME="$RUN_DIR/home"
ISOLATED_GOCACHE="$RUN_DIR/gocache"
ISOLATED_TMP="$RUN_DIR/tmp"

mkdir -p "$ISOLATED_HOME" "$ISOLATED_GOCACHE" "$ISOLATED_TMP"
chmod 700 "$ISOLATED_HOME" "$ISOLATED_GOCACHE" "$ISOLATED_TMP"

# Forward minimal git configuration for tools and tests requiring git identity
if [[ -n "$HOST_HOME" ]]; then
    if [[ -f "$HOST_HOME/.gitconfig" ]]; then
        cp "$HOST_HOME/.gitconfig" "$ISOLATED_HOME/.gitconfig" 2>/dev/null || true
    fi
    if [[ -d "$HOST_HOME/.config/git" ]]; then
        mkdir -p "$ISOLATED_HOME/.config"
        cp -R "$HOST_HOME/.config/git" "$ISOLATED_HOME/.config/git" 2>/dev/null || true
    fi
fi

export HOME="$ISOLATED_HOME"
export GOCACHE="$ISOLATED_GOCACHE"
export TMPDIR="$ISOLATED_TMP"

set +e
"$REPO_ROOT/scripts/preflight.sh" "$@"
EXIT_CODE=$?
set -e

exit "$EXIT_CODE"
