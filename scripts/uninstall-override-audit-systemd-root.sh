#!/bin/sh
set -eu
test "$#" = 1 && test "$0" = dear-agent-override-audit-systemd-uninstaller || exit 2
operator_user=$1; case "$operator_user" in *[!A-Za-z0-9._-]*|"") exit 2;; esac
IFS= read -r mode; case "$mode" in PROBE|UNINSTALL) ;; *) exit 2;; esac
test "$mode" != PROBE || exit 42
test "$(/usr/bin/id -u)" = 0 || exit 2
/usr/bin/systemctl disable --now "dear-agent-override-audit@$operator_user.timer" 2>/dev/null || true
/bin/rm -f /etc/systemd/system/dear-agent-override-audit@.service /etc/systemd/system/dear-agent-override-audit@.timer /usr/local/libexec/dear-agent-override-audit
/usr/bin/systemctl daemon-reload
