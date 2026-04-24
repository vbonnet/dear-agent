#!/usr/bin/env bash
# install-systemd.sh — install the agm-bus broker as a systemd user service
# that starts at login and restarts on crash.
#
# Usage:
#   ./install-systemd.sh install      # install + enable + start
#   ./install-systemd.sh uninstall    # stop + disable + remove unit
#   ./install-systemd.sh status       # show status and tail log
#
# Requires `agm-bus` on $PATH or at $HOME/go/bin. No root; the unit
# lands in ~/.config/systemd/user/ and runs as the current user.
#
# This script is a no-op when systemctl is not present (e.g. macOS), so
# it is safe to call from cross-platform install helpers.

set -euo pipefail

UNIT="agm-bus"
UNIT_DIR="$HOME/.config/systemd/user"
UNIT_PATH="$UNIT_DIR/${UNIT}.service"
LOG_DIR="$HOME/.agm/logs"
TEMPLATE="$(cd "$(dirname "$0")" && pwd)/agm-bus.service"

# Bail out silently when systemctl is not available (macOS, minimal containers).
if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemctl not found — skipping systemd install (use install-launchd.sh on macOS)" >&2
    exit 0
fi

usage() {
    cat <<EOF
Usage: $0 <install|uninstall|status>

  install     Expand the unit template, install it, and start the service.
  uninstall   Stop, disable, and remove the unit file.
  status      Report whether the service is active; tail the log.

Environment:
  AGM_BUS_BIN    Override agm-bus binary path (default: \$HOME/go/bin/agm-bus).
EOF
}

cmd_install() {
    if [[ ! -f "$TEMPLATE" ]]; then
        echo "error: unit template not found at $TEMPLATE" >&2
        exit 1
    fi
    local bin="${AGM_BUS_BIN:-$HOME/go/bin/agm-bus}"
    if [[ ! -x "$bin" ]]; then
        echo "error: agm-bus binary not found at $bin" >&2
        echo "install it first: GOWORK=off go install github.com/vbonnet/dear-agent/agm/cmd/agm-bus@latest" >&2
        exit 1
    fi

    mkdir -p "$UNIT_DIR" "$LOG_DIR"

    # Expand $HOME_DIR in the template so the unit has absolute paths.
    sed -e "s|\$HOME_DIR|$HOME|g" "$TEMPLATE" > "$UNIT_PATH"
    echo "installed $UNIT_PATH"

    systemctl --user daemon-reload
    systemctl --user enable "$UNIT"
    systemctl --user restart "$UNIT"
    echo "started $UNIT"
    cmd_status
}

cmd_uninstall() {
    if systemctl --user is-active --quiet "$UNIT" 2>/dev/null; then
        systemctl --user stop "$UNIT"
    fi
    if systemctl --user is-enabled --quiet "$UNIT" 2>/dev/null; then
        systemctl --user disable "$UNIT"
    fi
    if [[ -f "$UNIT_PATH" ]]; then
        rm -f "$UNIT_PATH"
        systemctl --user daemon-reload
        echo "removed $UNIT_PATH"
    else
        echo "not installed (no unit at $UNIT_PATH)"
    fi
}

cmd_status() {
    if systemctl --user is-active --quiet "$UNIT" 2>/dev/null; then
        echo "active: yes"
        local pid
        pid=$(systemctl --user show "$UNIT" --property=MainPID --value 2>/dev/null || true)
        if [[ -n "${pid:-}" && "$pid" != "0" ]]; then
            echo "pid: $pid"
        else
            echo "pid: (not running)"
        fi
    else
        echo "active: no"
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
