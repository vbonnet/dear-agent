#!/bin/sh
# gobin-guard-audit.sh — independent freshness check for gobin-guard.sh.
#
# This deliberately lives beside, but does not invoke, the primary guard. A
# separate launchd agent runs it every 90 seconds and alarms when the primary
# guard has not atomically refreshed its heartbeat within three minutes.

set -eu

PROG="gobin-guard-audit"
HOME_DIR="${HOME:-}"
[ -n "$HOME_DIR" ] || { echo "$PROG: HOME is not set" >&2; exit 2; }

heartbeat_path="${GOBIN_GUARD_HEARTBEAT:-$HOME_DIR/.local/state/dear-agent/gobin-guard.heartbeat}"
trail_path="${GOBIN_GUARD_TRAIL:-$HOME_DIR/.agm/vroom/trail.jsonl}"
max_age="${GOBIN_GUARD_MAX_AGE:-180}"

case "$max_age" in
*[!0-9]* | '') echo "$PROG: invalid GOBIN_GUARD_MAX_AGE: $max_age" >&2; exit 2 ;;
esac

now=$(date +%s)
last=$(cat "$heartbeat_path" 2>/dev/null || true)
reason=""
if [ -z "$last" ] || printf '%s' "$last" | grep -q '[^0-9]'; then
	reason="heartbeat is missing or invalid: $heartbeat_path"
elif [ "$last" -gt "$now" ] || [ $((now - last)) -gt "$max_age" ]; then
	reason="heartbeat is stale: $heartbeat_path"
fi

[ -z "$reason" ] && exit 0

trail_dir=$(dirname "$trail_path")
if mkdir -p "$trail_dir" 2>/dev/null; then
	(umask 0177 && touch "$trail_path" && chmod 600 "$trail_path") 2>/dev/null || true
	printf '{"timestamp":"%s","role":"watchdog","kind":"watchdog.gobin_guard.stale","payload":{"heartbeat":"%s","reason":"%s"}}\n' \
		"$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$heartbeat_path" "$reason" >>"$trail_path" 2>/dev/null || true
fi

if [ "${GOBIN_GUARD_NOTIFY:-1}" = "1" ] && [ "$(uname -s)" = "Darwin" ] && command -v osascript >/dev/null 2>&1; then
	osascript -e 'display notification "The GOBIN detector has stopped reporting. See gobin-guard-audit.err.log." with title "DEAR Agent GOBIN guard stale"' >/dev/null 2>&1 || true
fi
echo "$PROG: ALARM: $reason" >&2
exit 1
