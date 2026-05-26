#!/usr/bin/env bash
#
# bumblebee-scan.sh — run Bumblebee with the dear-agent defaults and write
# NDJSON to the per-user data dir.
#
# Output: $XDG_DATA_HOME/dear-agent/bumblebee/<YYYY-MM-DD>.ndjson on Linux,
#         ~/Library/Application Support/dear-agent/bumblebee/<YYYY-MM-DD>.ndjson on macOS.
# Local-only, atomic write (temp + rename) — no network egress beyond what
# Bumblebee itself does (catalog fetch only, and only if you pass one).
#
# Catalog: if BUMBLEBEE_CATALOG points at a file, or if etc/bumblebee/catalog.json
# exists at the repo root, it's passed as --exposure-catalog. Otherwise the
# scan runs in inventory-only mode (still useful: diff-vs-yesterday reveals
# new MCP/IDE/browser-extension installs).
#
# Usage:
#   scripts/bumblebee-scan.sh                       # project profile, default output
#   scripts/bumblebee-scan.sh --profile baseline    # narrower scope
#   scripts/bumblebee-scan.sh --profile deep --root "$HOME"
#   scripts/bumblebee-scan.sh --output /path/to.ndjson

set -euo pipefail

PROFILE="project"
ROOT=""
OUTPUT=""
EXTRA_ARGS=()

while [ $# -gt 0 ]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2 ;;
    --root)    ROOT="$2"; shift 2 ;;
    --output)  OUTPUT="$2"; shift 2 ;;
    -h|--help) sed -n '3,21p' "$0"; exit 0 ;;
    *)         EXTRA_ARGS+=("$1"); shift ;;
  esac
done

# Resolve binary: $BUMBLEBEE_BIN > PATH > default install location.
if [ -n "${BUMBLEBEE_BIN:-}" ] && [ -x "${BUMBLEBEE_BIN}" ]; then
  bin="$BUMBLEBEE_BIN"
elif command -v bumblebee >/dev/null 2>&1; then
  bin="$(command -v bumblebee)"
elif [ -x "$HOME/.local/bin/bumblebee" ]; then
  bin="$HOME/.local/bin/bumblebee"
else
  echo "bumblebee not found. Run \`make bumblebee-install\` first." >&2
  exit 1
fi

# Default output dir follows platform conventions.
if [ -z "$OUTPUT" ]; then
  case "$(uname -s)" in
    Darwin)
      out_dir="$HOME/Library/Application Support/dear-agent/bumblebee"
      ;;
    Linux)
      out_dir="${XDG_DATA_HOME:-$HOME/.local/share}/dear-agent/bumblebee"
      ;;
    *)
      out_dir="$HOME/.dear-agent/bumblebee"
      ;;
  esac
  mkdir -p "$out_dir"
  OUTPUT="$out_dir/$(date -u +%Y-%m-%d).ndjson"
fi

# Catalog autodiscovery — repo-local catalog at etc/bumblebee/catalog.json
# wins over the env override only if env isn't set.
catalog=""
if [ -n "${BUMBLEBEE_CATALOG:-}" ] && [ -f "${BUMBLEBEE_CATALOG}" ]; then
  catalog="$BUMBLEBEE_CATALOG"
else
  # Best-effort repo-root resolution. If we're not in a git tree (running under
  # launchd outside the worktree, for example) just skip.
  if repo_root="$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel 2>/dev/null)"; then
    if [ -f "$repo_root/etc/bumblebee/catalog.json" ]; then
      catalog="$repo_root/etc/bumblebee/catalog.json"
    fi
  fi
fi

args=(scan --profile "$PROFILE")
[ -n "$ROOT" ]    && args+=(--root "$ROOT")
[ -n "$catalog" ] && args+=(--exposure-catalog "$catalog")
[ "${#EXTRA_ARGS[@]}" -gt 0 ] && args+=("${EXTRA_ARGS[@]}")

# Atomic write: scan to temp, rename on success. Prevents a half-written file
# from being mistaken for a complete inventory by the next diff pass.
tmp_out="$(mktemp "${OUTPUT}.XXXXXX")"
trap 'rm -f "$tmp_out"' EXIT

echo "[bumblebee-scan] profile=$PROFILE${catalog:+ catalog=$catalog} → $OUTPUT" >&2
"$bin" "${args[@]}" > "$tmp_out"
mv "$tmp_out" "$OUTPUT"
trap - EXIT

# One-line summary to stderr so launchd logs are useful.
records="$(wc -l < "$OUTPUT" | tr -d ' ')"
echo "[bumblebee-scan] wrote $records records to $OUTPUT" >&2
