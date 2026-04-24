#!/usr/bin/env bash
# install-launchd.sh — install the agm-bus broker as a macOS launchd user
# agent that starts on login and restarts on crash.
#
# Usage:
#   ./install-launchd.sh install      # install + load
#   ./install-launchd.sh uninstall    # unload + remove
#   ./install-launchd.sh status       # show status (loaded / pid / log tail)
#
# Requires `agm-bus` on $PATH or at $HOME/go/bin. No root; the plist
# lands in ~/Library/LaunchAgents and runs as the current user.

set -euo pipefail

LABEL="com.vbonnet.agm-bus"
AGENT_DIR="$HOME/Library/LaunchAgents"
PLIST_PATH="$AGENT_DIR/${LABEL}.plist"
LOG_DIR="$HOME/.agm/logs"
TEMPLATE="$(cd "$(dirname "$0")" && pwd)/com.vbonnet.agm-bus.plist"

usage() {
    cat <<EOF
Usage: $0 <install|uninstall|status>

  install     Install ~/Library/LaunchAgents/${LABEL}.plist and bootstrap it.
  uninstall   Bootout the agent and remove the plist.
  status      Report whether the agent is loaded; tail the log.

Environment:
  AGM_BUS_BIN    Override agm-bus binary path (default: $HOME/go/bin/agm-bus).
EOF
}

cmd_install() {
    if [[ ! -f "$TEMPLATE" ]]; then
        echo "error: template plist not found at $TEMPLATE" >&2
        exit 1
    fi
    local bin="${AGM_BUS_BIN:-$HOME/go/bin/agm-bus}"
    if [[ ! -x "$bin" ]]; then
        echo "error: agm-bus binary not found at $bin" >&2
        echo "install it first: GOWORK=off go install github.com/vbonnet/dear-agent/agm/cmd/agm-bus@latest" >&2
        exit 1
    fi

    mkdir -p "$AGENT_DIR" "$LOG_DIR"

    # Expand $HOME_DIR in the template so the plist has absolute paths.
    # Using sed rather than envsubst so we don't require the gettext package.
    sed -e "s|\$HOME_DIR|$HOME|g" "$TEMPLATE" > "$PLIST_PATH"
    echo "installed $PLIST_PATH"

    # Bootout first in case an older copy is loaded; ignore "not found" exits.
    launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
    launchctl bootstrap "gui/$(id -u)" "$PLIST_PATH"
    launchctl enable "gui/$(id -u)/$LABEL"
    echo "loaded $LABEL"
    sleep 1
    cmd_status
}

cmd_uninstall() {
    if [[ -f "$PLIST_PATH" ]]; then
        launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
        rm -f "$PLIST_PATH"
        echo "removed $PLIST_PATH"
    else
        echo "not installed (no plist at $PLIST_PATH)"
    fi
}

cmd_status() {
    if launchctl print "gui/$(id -u)/$LABEL" >/dev/null 2>&1; then
        echo "loaded: yes"
        local pid
        pid=$(launchctl print "gui/$(id -u)/$LABEL" | awk -F' = ' '/"pid" =/ {print $2; exit}')
        if [[ -n "${pid:-}" && "$pid" != "0" ]]; then
            echo "pid: $pid"
        else
            echo "pid: (not running)"
        fi
    else
        echo "loaded: no"
    fi
    if [[ -f "$LOG_DIR/agm-bus.log" ]]; then
        echo "--- tail of $LOG_DIR/agm-bus.log ---"
        tail -n 20 "$LOG_DIR/agm-bus.log"
    fi
}

case "${1:-}" in
    install)    cmd_install ;;
    uninstall)  cmd_uninstall ;;
    status)     cmd_status ;;
    -h|--help|help|"") usage ;;
    *) echo "unknown command: $1" >&2; usage; exit 2 ;;
esac
