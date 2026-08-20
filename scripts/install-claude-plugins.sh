#!/usr/bin/env bash
# Install dear-agent Claude Code plugins (agm, wayfinder, youtube, research-pipeline).
#
# This script registers the dear-agent marketplace (from a local repo checkout
# or from GitHub) and installs every plugin it declares. It is idempotent:
# running it twice on an up-to-date repo is a no-op.
#
# Usage:
#   scripts/install-claude-plugins.sh                # install from this repo (default)
#   scripts/install-claude-plugins.sh --github       # install from github.com/vbonnet/dear-agent
#   scripts/install-claude-plugins.sh --dry-run      # print actions without running them
#   scripts/install-claude-plugins.sh --uninstall    # uninstall every dear-agent plugin
#   scripts/install-claude-plugins.sh --scope user   # pass --scope to `claude plugin install`
#
# Environment overrides (mainly for tests):
#   CLAUDE_BIN           path to the claude CLI (default: claude)
#   DEAR_AGENT_REPO      repo root to use (default: directory containing this script's ../)
#   DEAR_AGENT_GH_REPO   GitHub repo coordinate for --github (default: vbonnet/dear-agent)
#   MARKETPLACE_NAME     marketplace name (default: parsed from marketplace.json)

set -euo pipefail

# ----- bootstrap -----------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT_DEFAULT="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="${DEAR_AGENT_REPO:-$REPO_ROOT_DEFAULT}"
CLAUDE_BIN="${CLAUDE_BIN:-claude}"
GH_REPO="${DEAR_AGENT_GH_REPO:-vbonnet/dear-agent}"

SOURCE_MODE="local"
DRY_RUN=0
UNINSTALL=0
SCOPE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --github)    SOURCE_MODE="github"; shift ;;
    --local)     SOURCE_MODE="local";  shift ;;
    --dry-run)   DRY_RUN=1;            shift ;;
    --uninstall) UNINSTALL=1;          shift ;;
    --scope)     SCOPE="$2";           shift 2 ;;
    --scope=*)   SCOPE="${1#*=}";      shift ;;
    -h|--help)
      sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      exit 2 ;;
  esac
done

# ----- helpers -------------------------------------------------------------

log()  { printf '%s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }
die()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

run() {
  if [[ $DRY_RUN -eq 1 ]]; then
    printf '+ %s\n' "$*"
  else
    "$@"
  fi
}

require_claude() {
  if ! command -v "$CLAUDE_BIN" >/dev/null 2>&1; then
    die "claude CLI not found (looked for: $CLAUDE_BIN). Install Claude Code first: https://claude.com/code"
  fi
}

require_marketplace_manifest() {
  local manifest="$REPO_ROOT/.claude-plugin/marketplace.json"
  [[ -f "$manifest" ]] || die "marketplace manifest not found at: $manifest"
}

# Parse plugin names from .claude-plugin/marketplace.json without jq.
# Looks for `"name": "<name>"` lines inside the plugins[] block.
list_plugin_names() {
  local manifest="$REPO_ROOT/.claude-plugin/marketplace.json"
  awk '
    /"plugins"[[:space:]]*:[[:space:]]*\[/ { in_plugins=1; next }
    in_plugins && /\]/                     { in_plugins=0 }
    in_plugins && /"name"[[:space:]]*:/ {
      match($0, /"name"[[:space:]]*:[[:space:]]*"[^"]+"/)
      if (RSTART) {
        s = substr($0, RSTART, RLENGTH)
        sub(/.*"name"[[:space:]]*:[[:space:]]*"/, "", s)
        sub(/".*/, "", s)
        print s
      }
    }
  ' "$manifest"
}

marketplace_name() {
  if [[ -n "${MARKETPLACE_NAME:-}" ]]; then
    printf '%s\n' "$MARKETPLACE_NAME"
    return
  fi
  local manifest="$REPO_ROOT/.claude-plugin/marketplace.json"
  awk '
    /^[[:space:]]*"name"[[:space:]]*:/ && !seen {
      match($0, /"name"[[:space:]]*:[[:space:]]*"[^"]+"/)
      if (RSTART) {
        s = substr($0, RSTART, RLENGTH)
        sub(/.*"name"[[:space:]]*:[[:space:]]*"/, "", s)
        sub(/".*/, "", s)
        print s
        seen = 1
      }
    }
  ' "$manifest"
}

marketplace_is_known() {
  local name="$1"
  local out
  out="$("$CLAUDE_BIN" plugin marketplace list 2>/dev/null || true)"
  printf '%s\n' "$out" | awk -v n="$name" '$1=="❯" && $2==n { found=1 } END { exit found ? 0 : 1 }'
}

plugin_is_installed() {
  local spec="$1"
  local out
  out="$("$CLAUDE_BIN" plugin list 2>/dev/null || true)"
  printf '%s\n' "$out" | awk -v s="$spec" '$1=="❯" && $2==s { found=1 } END { exit found ? 0 : 1 }'
}

# ----- main ----------------------------------------------------------------

require_claude
require_marketplace_manifest

MARKET="$(marketplace_name)"
[[ -n "$MARKET" ]] || die "could not parse marketplace name from .claude-plugin/marketplace.json"

# Bash 3.2-compatible array population (no mapfile).
PLUGINS=()
while IFS= read -r _name; do
  [[ -n "$_name" ]] && PLUGINS+=("$_name")
done < <(list_plugin_names)
[[ ${#PLUGINS[@]} -gt 0 ]] || die "no plugins found in marketplace manifest"

case "$SOURCE_MODE" in
  local)  SOURCE_ARG="$REPO_ROOT" ;;
  github) SOURCE_ARG="$GH_REPO"   ;;
  *)      die "unknown source mode: $SOURCE_MODE" ;;
esac

log "dear-agent plugin install"
log "  repo:        $REPO_ROOT"
log "  marketplace: $MARKET ($SOURCE_MODE → $SOURCE_ARG)"
log "  plugins:     ${PLUGINS[*]}"
[[ -n "$SCOPE" ]] && log "  scope:       $SCOPE"
[[ $DRY_RUN -eq 1 ]] && log "  mode:        DRY RUN"
log ""

if [[ $UNINSTALL -eq 1 ]]; then
  log "Uninstalling plugins..."
  for p in "${PLUGINS[@]}"; do
    run "$CLAUDE_BIN" plugin uninstall "${p}@${MARKET}" || warn "uninstall $p failed (already removed?)"
  done
  log "Done."
  exit 0
fi

# 1. Register or update the marketplace.
if marketplace_is_known "$MARKET"; then
  log "Marketplace '$MARKET' already registered; refreshing from source..."
  run "$CLAUDE_BIN" plugin marketplace update "$MARKET"
else
  log "Registering marketplace '$MARKET' from $SOURCE_ARG..."
  run "$CLAUDE_BIN" plugin marketplace add "$SOURCE_ARG"
fi

# 2. Install (or update) each plugin.
for p in "${PLUGINS[@]}"; do
  spec="${p}@${MARKET}"
  if plugin_is_installed "$spec"; then
    log "Updating $spec..."
    if [[ -n "$SCOPE" ]]; then
      run "$CLAUDE_BIN" plugin update "$spec" --scope "$SCOPE" || warn "update $spec failed"
    else
      run "$CLAUDE_BIN" plugin update "$spec" || warn "update $spec failed"
    fi
  else
    log "Installing $spec..."
    if [[ -n "$SCOPE" ]]; then
      run "$CLAUDE_BIN" plugin install "$spec" --scope "$SCOPE"
    else
      run "$CLAUDE_BIN" plugin install "$spec"
    fi
  fi
done

log ""
log "✔ done. Run 'claude plugin list' to verify, and restart Claude Code to load."
