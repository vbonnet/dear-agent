#!/bin/sh
set -eu
test "$#" = 0 && test "$0" = dear-agent-override-audit-launchdaemon-uninstaller || exit 2
IFS= read -r mode; case "$mode" in PROBE|UNINSTALL) ;; *) exit 2;; esac
test "$mode" != PROBE || exit 42
test "$(/usr/bin/id -u)" = 0 || exit 2
/bin/launchctl bootout system/com.dear-agent.override-audit 2>/dev/null || true
/bin/rm -f /Library/LaunchDaemons/com.dear-agent.override-audit.plist /usr/local/libexec/dear-agent-override-audit
