#!/usr/bin/env bash
# Install every Claude Code plugin declared by the dear-agent marketplace.
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

# Parse the root marketplace name and direct plugin-object names together
# without adding a JSON runtime dependency. Names containing JSON escapes are
# rejected rather than decoded ambiguously; escapes remain valid in other text.
parse_marketplace_manifest() {
  local manifest="$REPO_ROOT/.claude-plugin/marketplace.json"
  awk '
    function invalid() {
      invalid_json = 1
      exit 1
    }
    {
      for (i = 1; i <= length($0); i++) {
        ch = substr($0, i, 1)
        if (in_string) {
          if (escaped) {
            token = token ch
            escaped = 0
          } else if (ch == "\\") {
            escaped = 1
            string_escaped = 1
          } else if (ch == "\"") {
            in_string = 0
            if (want_marketplace_name) {
              if (token == "" || string_escaped) invalid()
              marketplace_name = token
              marketplace_names++
              want_marketplace_name = 0
            } else if (want_plugin_name) {
              if (token == "" || string_escaped || seen_plugin_name[token]) invalid()
              seen_plugin_name[token] = 1
              plugin_name[++plugin_names] = token
              direct_plugin_names++
              want_plugin_name = 0
            }
            last_string = token
            last_string_escaped = string_escaped
            have_string = 1
          } else {
            token = token ch
          }
          continue
        }
        if (ch ~ /[[:space:]]/) continue
        if (ch == "\"") {
          if (want_plugins_array) invalid()
          in_string = 1
          escaped = 0
          string_escaped = 0
          token = ""
          continue
        }
        if ((want_plugins_array && ch != "[") || want_marketplace_name || want_plugin_name) invalid()
        if (ch == ":") {
          if (!have_string) invalid()
          if (!in_plugins && depth == 1 && last_string == "name") {
            if (last_string_escaped || marketplace_names || want_marketplace_name) invalid()
            want_marketplace_name = 1
          } else if (!in_plugins && depth == 1 && last_string == "plugins") {
            if (last_string_escaped || saw_plugins) invalid()
            want_plugins_array = 1
          } else if (direct_plugin && depth == direct_plugin_depth && last_string == "name") {
            if (last_string_escaped || direct_plugin_names || want_plugin_name) invalid()
            want_plugin_name = 1
          }
          have_string = 0
        } else if (ch == "[") {
          if (in_plugins && depth == plugins_depth) invalid()
          depth++
          if (want_plugins_array) {
            in_plugins = 1
            plugins_depth = depth
            plugin_array_has_value = 0
            want_plugins_array = 0
          }
          have_string = 0
        } else if (ch == "]") {
          if (depth < 1) invalid()
          if (in_plugins && depth == plugins_depth) {
            if (direct_plugin || (plugin_names && !plugin_array_has_value)) invalid()
            in_plugins = 0
            saw_plugins = 1
          }
          depth--
          have_string = 0
        } else if (ch == "{") {
          if (in_plugins && depth == plugins_depth) {
            if (direct_plugin || plugin_array_has_value) invalid()
            direct_plugin = 1
            direct_plugin_depth = depth + 1
            direct_plugin_names = 0
          }
          depth++
          have_string = 0
        } else if (ch == "}") {
          if (depth < 1) invalid()
          if (direct_plugin && depth == direct_plugin_depth) {
            if (direct_plugin_names != 1) invalid()
            direct_plugin = 0
            plugin_array_has_value = 1
          }
          depth--
          have_string = 0
        } else if (ch == ",") {
          if (in_plugins && depth == plugins_depth) {
            if (direct_plugin || !plugin_array_has_value) invalid()
            plugin_array_has_value = 0
          }
          have_string = 0
        } else {
          have_string = 0
        }
      }
    }
    END {
      if (invalid_json || in_string || escaped || depth != 0 || in_plugins || direct_plugin || want_plugins_array || want_marketplace_name || want_plugin_name || marketplace_names != 1 || !saw_plugins || plugin_names == 0) exit 1
      printf "marketplace\t%s\n", marketplace_name
      for (i = 1; i <= plugin_names; i++) printf "plugin\t%s\n", plugin_name[i]
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

MANIFEST_ENTRIES="$(parse_marketplace_manifest)" || die "could not parse an unambiguous marketplace name and complete plugin set from marketplace manifest"
PARSED_MARKET=""
PLUGINS=()
while IFS=$'\t' read -r _kind _name; do
  case "$_kind" in
    marketplace) PARSED_MARKET="$_name" ;;
    plugin)      PLUGINS+=("$_name") ;;
    *)           die "marketplace manifest scanner returned an unknown entry" ;;
  esac
done <<<"$MANIFEST_ENTRIES"
MARKET="${MARKETPLACE_NAME:-$PARSED_MARKET}"
[[ -n "$MARKET" && -n "$PARSED_MARKET" && ${#PLUGINS[@]} -gt 0 ]] || die "marketplace manifest has no usable marketplace or plugin names"

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
      run "$CLAUDE_BIN" plugin update "$spec" --scope "$SCOPE" || die "update $spec failed; refusing to report success while a declared plugin may be stale"
    else
      run "$CLAUDE_BIN" plugin update "$spec" || die "update $spec failed; refusing to report success while a declared plugin may be stale"
    fi
  else
    log "Installing $spec..."
    if [[ -n "$SCOPE" ]]; then
      run "$CLAUDE_BIN" plugin install "$spec" --scope "$SCOPE" || die "install $spec failed; refusing to report success while a declared plugin is missing"
    else
      run "$CLAUDE_BIN" plugin install "$spec" || die "install $spec failed; refusing to report success while a declared plugin is missing"
    fi
  fi
done

log ""
log "✔ done. Run 'claude plugin list' to verify, and restart Claude Code to load."
