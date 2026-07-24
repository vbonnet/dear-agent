#!/bin/sh
# Independent freshness audit for gobin-guard.sh; invoked by its own launchd job.
set -eu
home=${HOME:?HOME is not set}; heartbeat=${GOBIN_GUARD_HEARTBEAT:-$home/.local/state/dear-agent/gobin-guard.heartbeat}
trail=${GOBIN_GUARD_TRAIL:-$home/.agm/vroom/trail.jsonl}; max_age=${GOBIN_GUARD_MAX_AGE:-180}
alarm=${GOBIN_GUARD_AUDIT_ALARM_STATE:-$home/.local/state/dear-agent/gobin-guard-audit.alarm}
case "$max_age" in *[!0-9]*|''|???????????*) echo "gobin-guard-audit: invalid GOBIN_GUARD_MAX_AGE" >&2; exit 2;; esac
now=$(date +%s); last=$(cat "$heartbeat" 2>/dev/null || true); reason=
case "$last" in ''|*[!0-9]*|0[0-9]*|???????????*) reason="heartbeat is missing or invalid: $heartbeat";; *) [ "$last" -gt "$now" ] || [ $((now-last)) -gt "$max_age" ] && reason="heartbeat is stale: $heartbeat";; esac
if [ -z "$reason" ]; then rm -f "$alarm" 2>/dev/null || true; exit 0; fi
delivered=1
trail_dir=$(dirname "$trail")
if mkdir -p "$trail_dir" 2>/dev/null; then
  (umask 0177 && touch "$trail" && chmod 600 "$trail") 2>/dev/null || true
  event_id="gobin-guard-audit-$(date -u +%Y%m%d%H%M%S)-$$"
  if [ ! -e "$alarm" ] && printf '{"event_id":"%s","timestamp":"%s","role":"watchdog","kind":"watchdog.gobin_guard.stale","payload":{"heartbeat":"%s","reason":"%s"}}\n' "$event_id" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$heartbeat" "$reason" >>"$trail" 2>/dev/null; then delivered=0; fi
fi
if [ ! -e "$alarm" ] && [ "${GOBIN_GUARD_NOTIFY:-1}" = 1 ] && [ "$(uname -s)" = Darwin ] && command -v osascript >/dev/null 2>&1; then
  osascript -e 'display notification "The GOBIN detector has stopped reporting. See gobin-guard-audit.err.log." with title "DEAR Agent GOBIN guard stale"' >/dev/null 2>&1 && delivered=0 || true
fi
if [ "$delivered" -eq 0 ]; then (umask 0177 && mkdir -p "$(dirname "$alarm")" && : >"$alarm") 2>/dev/null || true; fi
echo "gobin-guard-audit: ALARM: $reason" >&2; exit 1
