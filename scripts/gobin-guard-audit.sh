#!/bin/sh
# Independent freshness audit for gobin-guard.sh; invoked by its own launchd job.
set -eu
home=${HOME:?HOME is not set}; heartbeat=${GOBIN_GUARD_HEARTBEAT:-$home/.local/state/dear-agent/gobin-guard.heartbeat}
trail=${GOBIN_GUARD_TRAIL:-$home/.agm/vroom/trail.jsonl}; max_age=${GOBIN_GUARD_MAX_AGE:-180}
case "$max_age" in *[!0-9]*|'') echo "gobin-guard-audit: invalid GOBIN_GUARD_MAX_AGE" >&2; exit 2;; esac
now=$(date +%s); last=$(cat "$heartbeat" 2>/dev/null || true); reason=
case "$last" in ''|*[!0-9]*) reason="heartbeat is missing or invalid: $heartbeat";; *) [ "$last" -gt "$now" ] || [ $((now-last)) -gt "$max_age" ] && reason="heartbeat is stale: $heartbeat";; esac
[ -z "$reason" ] && exit 0
trail_dir=$(dirname "$trail")
if mkdir -p "$trail_dir" 2>/dev/null; then
  (umask 0177 && touch "$trail" && chmod 600 "$trail") 2>/dev/null || true
  printf '{"timestamp":"%s","role":"watchdog","kind":"watchdog.gobin_guard.stale","payload":{"heartbeat":"%s","reason":"%s"}}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$heartbeat" "$reason" >>"$trail" 2>/dev/null || true
fi
if [ "${GOBIN_GUARD_NOTIFY:-1}" = 1 ] && [ "$(uname -s)" = Darwin ] && command -v osascript >/dev/null 2>&1; then
  osascript -e 'display notification "The GOBIN detector has stopped reporting. See gobin-guard-audit.err.log." with title "DEAR Agent GOBIN guard stale"' >/dev/null 2>&1 || true
fi
echo "gobin-guard-audit: ALARM: $reason" >&2
exit 1
