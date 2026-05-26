#!/usr/bin/env bash
#
# install-bumblebee-launchagent.sh — install (or remove) the daily Bumblebee
# endpoint-scan LaunchAgent at ~/Library/LaunchAgents/com.dear-agent.bumblebee.plist.
#
# Why LaunchAgent (per-user), not LaunchDaemon (root): Bumblebee inventories
# the developer endpoint — npm/PyPI lockfiles, MCP configs, IDE & browser
# extensions — and "per-user" is the right scope for that. Running as root
# would broaden scope unnecessarily and create a service we'd have to reason
# about under sudo.
#
# Idempotent. Re-running this command refreshes the plist (path substitutions,
# schedule) and reloads the agent.
#
# Usage:
#   scripts/install-bumblebee-launchagent.sh             # install
#   scripts/install-bumblebee-launchagent.sh --uninstall # remove
#   scripts/install-bumblebee-launchagent.sh --status    # show launchctl state

set -euo pipefail

if [ "$(uname -s)" != "Darwin" ]; then
  echo "LaunchAgent is macOS-only. On Linux, use systemd --user or cron." >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEMPLATE="$REPO_ROOT/etc/launchd/com.dear-agent.bumblebee.plist"
TARGET="$HOME/Library/LaunchAgents/com.dear-agent.bumblebee.plist"
LABEL="com.dear-agent.bumblebee"
ACTION="install"

while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall) ACTION="uninstall"; shift ;;
    --status)    ACTION="status"; shift ;;
    -h|--help)   sed -n '3,22p' "$0"; exit 0 ;;
    *)           echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

uid="$(id -u)"
domain="gui/$uid"

case "$ACTION" in
  status)
    if launchctl print "$domain/$LABEL" 2>/dev/null | head -20; then
      :
    else
      echo "$LABEL is not loaded in $domain"
      exit 1
    fi
    exit 0
    ;;

  uninstall)
    # bootout is the load-counterpart of bootstrap. Returns non-zero if the
    # service wasn't loaded, which is fine on uninstall — swallow it.
    launchctl bootout "$domain/$LABEL" 2>/dev/null || true
    if [ -f "$TARGET" ]; then
      rm -f "$TARGET"
      echo "Removed $TARGET"
    else
      echo "Not installed at $TARGET (nothing to do)"
    fi
    exit 0
    ;;
esac

# install
if [ ! -f "$TEMPLATE" ]; then
  echo "missing template: $TEMPLATE" >&2
  exit 1
fi

# Ensure log dir exists — launchd will not create intermediate directories
# for StandardOutPath/StandardErrorPath.
mkdir -p "$HOME/Library/Logs/dear-agent"
mkdir -p "$HOME/Library/LaunchAgents"

# Substitute placeholders. We use sed with `|` delimiter because the
# substituted paths contain `/`. Escape `&` in HOME just in case (unlikely
# but cheap).
home_escaped="${HOME//&/\\&}"
repo_escaped="${REPO_ROOT//&/\\&}"
sed \
  -e "s|__USER_HOME__|${home_escaped}|g" \
  -e "s|__REPO_ROOT__|${repo_escaped}|g" \
  "$TEMPLATE" > "$TARGET"
chmod 0644 "$TARGET"

# Reload: bootout the old (if any), bootstrap the new. bootstrap fails if the
# label is already loaded, so the bootout-first sequence is required for
# idempotent reinstall.
launchctl bootout "$domain/$LABEL" 2>/dev/null || true
launchctl bootstrap "$domain" "$TARGET"

echo "Installed $LABEL"
echo "  plist:   $TARGET"
echo "  runs:    daily at 04:00 local"
echo "  logs:    $HOME/Library/Logs/dear-agent/bumblebee.log"
echo "  output:  $HOME/Library/Application Support/dear-agent/bumblebee/<date>.ndjson"
echo
echo "Run on demand:   launchctl kickstart $domain/$LABEL"
echo "Uninstall:       scripts/install-bumblebee-launchagent.sh --uninstall"
