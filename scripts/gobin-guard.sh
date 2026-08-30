#!/bin/sh
# gobin-guard.sh — SENSE + ESCALATE guard for the Go toolchain bin directory.
#
# Bead ce-24f1: on 2026-07-15 the entire ~/go/bin directory was deleted,
# silently taking down agm/vroom-dispatch/mergeloop/disk-watchdog and halting
# VROOM mesh dispatch. The only signals were reactive, per-binary launchd
# "No such file or directory" errors that surfaced hours later.
#
# This guard makes "the whole GOBIN disappeared" a first-class, cheap SENSE
# condition. On every tick it checks that:
#   1. the GOBIN directory exists, and
#   2. a sentinel binary inside it (agm, by default) is an executable file.
# On failure it ESCALATEs by appending one watchdog.gobin.missing record to the
# VROOM decision trail (the same JSONL the disk-watchdog and Overseer write to),
# prints to stderr, and posts a macOS notification from its launchd agent, then
# exits non-zero.
#
# It is deliberately a dependency-free POSIX shell script installed OUTSIDE
# ~/go/bin (see the launchd plist / Makefile install target): a compiled
# watchdog living in ~/go/bin would share the fate of what it guards and be
# deleted by the very event it is meant to catch.
#
# This guard only DETECTS and ESCALATEs. It never rebuilds or deletes anything;
# remediation (rebuilding the toolchain) is a human/supervisor decision.
#
# Environment overrides:
#   GOBIN_GUARD_DIR      GOBIN directory to check   (default: $HOME/go/bin)
#   GOBIN_GUARD_BINARY   sentinel executable name   (default: agm)
#   GOBIN_GUARD_TRAIL    decision-trail JSONL path  (default: $HOME/.agm/vroom/trail.jsonl)
#   GOBIN_GUARD_HEARTBEAT freshness record path     (default: $HOME/.local/state/dear-agent/gobin-guard.heartbeat)
#   GOBIN_GUARD_ROLE     role recorded in the trail (default: watchdog)
#   GOBIN_GUARD_NOTIFY   set to 0 to suppress the macOS notification (default: 1)
#
# Flags:
#   --json     emit a machine-readable status object to stdout
#   --quiet    suppress the healthy-case heartbeat line on stdout
#   -h|--help  usage
#
# Exit codes: 0 = healthy; 1 = GOBIN missing/degraded (alarm escalated);
#             2 = usage error.

set -eu

PROG="gobin-guard"

usage() {
	sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'
}

json_output=0
quiet=0

while [ $# -gt 0 ]; do
	case "$1" in
	--json) json_output=1 ;;
	--quiet) quiet=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "$PROG: unknown argument: $1" >&2
		exit 2
		;;
	esac
	shift
done

HOME_DIR="${HOME:-}"
if [ -z "$HOME_DIR" ]; then
	echo "$PROG: HOME is not set" >&2
	exit 2
fi

gobin_dir="${GOBIN_GUARD_DIR:-$HOME_DIR/go/bin}"
sentinel_name="${GOBIN_GUARD_BINARY:-agm}"
sentinel_path="$gobin_dir/$sentinel_name"
trail_path="${GOBIN_GUARD_TRAIL:-$HOME_DIR/.agm/vroom/trail.jsonl}"
heartbeat_path="${GOBIN_GUARD_HEARTBEAT:-$HOME_DIR/.local/state/dear-agent/gobin-guard.heartbeat}"
alarm_path="${GOBIN_GUARD_ALARM_STATE:-$HOME_DIR/.local/state/dear-agent/gobin-guard.alarm}"
role="${GOBIN_GUARD_ROLE:-watchdog}"

# Keep option-looking relative paths from being parsed as utility flags. Shell
# redirections accept the original spelling, but dirname/rm do not reliably do
# so across the supported host utilities.
case "$alarm_path" in
-*) alarm_path="./$alarm_path" ;;
esac

# Repair state directories left non-searchable by the former alarm umask. A
# subshell gives each recursive call private variables in POSIX sh.
ensure_searchable_dir() (
	dir=$1
	parent=$(dirname "$dir")
	if [ "$parent" != "$dir" ]; then
		ensure_searchable_dir "$parent" || return 1
	fi
	# Never repair through a link to a target outside the lexical state tree.
	# chmod u+x enforces the owner bit when permitted; the fallback accepts an
	# already-traversable non-owned ancestor. test -x alone is insufficient
	# because root reports effective access for mode-0600 directories.
	if [ -L "$dir" ]; then
		return 1
	elif [ -d "$dir" ]; then
		chmod u+x "$dir" 2>/dev/null || [ -x "$dir" ]
	elif [ -e "$dir" ]; then
		return 1
	else
		(umask 0077 && mkdir "$dir")
	fi
)

is_alarm_marker() {
	[ ! -L "$1" ] && [ -f "$1" ]
}

persist_alarm_marker() (
	marker_path=$1
	ensure_searchable_dir "$(dirname "$marker_path")" || return 1
	# Reject every existing leaf before redirection so FIFOs and devices are
	# never opened. Noclobber also rejects a regular file or link inserted after
	# this serialized check; ce-1hu9.108 owns broader concurrent replacement.
	[ ! -e "$marker_path" ] && [ ! -L "$marker_path" ] || return 1
	set -C
	umask 0077
	: >"$marker_path"
)

# A distinct launchd agent audits this bounded freshness record. Write it before
# classification so a failing detector is distinguishable from an unloaded or
# missing detector; use rename so the auditor never reads a partial timestamp.
write_heartbeat() {
	heartbeat_dir=$(dirname "$heartbeat_path")
	if ! ensure_searchable_dir "$heartbeat_dir" 2>/dev/null; then
		echo "$PROG: warning: cannot create heartbeat dir $heartbeat_dir" >&2
		return 0
	fi
	tmp=$(mktemp "$heartbeat_dir/.gobin-guard.heartbeat.XXXXXX" 2>/dev/null) || {
		echo "$PROG: warning: cannot create heartbeat" >&2
		return 0
	}
	(umask 0177 && printf '%s\n' "$(date +%s)" >"$tmp" && chmod 600 "$tmp" && mv -f "$tmp" "$heartbeat_path") || {
		rm -f "$tmp"
		echo "$PROG: warning: cannot publish heartbeat" >&2
	}
}

write_heartbeat

# Classify the GOBIN state.
status="ok"
reason=""
if [ ! -d "$gobin_dir" ]; then
	status="missing_dir"
	reason="GOBIN directory does not exist: $gobin_dir"
elif [ ! -e "$sentinel_path" ]; then
	status="missing_sentinel"
	reason="sentinel binary is absent: $sentinel_path"
elif [ ! -f "$sentinel_path" ] || [ ! -x "$sentinel_path" ]; then
	status="sentinel_not_executable"
	reason="sentinel exists but is not an executable file: $sentinel_path"
fi

# json_escape escapes backslashes and double quotes for embedding a value in a
# JSON string. Paths are the only dynamic content and never contain control
# characters in practice; this keeps the guard dependency-free.
json_escape() {
	printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

now_utc() {
	date -u +%Y-%m-%dT%H:%M:%SZ
}

new_event_id() {
	if command -v uuidgen >/dev/null 2>&1; then
		uuidgen | tr '[:upper:]' '[:lower:]'
	else
		printf 'gobin-guard-%s-%s' "$(date -u +%Y%m%d%H%M%S)" "$$"
	fi
}

escalate() {
	# Append one decision-trail record. Best-effort: a trail write failure must
	# not mask the alarm exit code.
	trail_dir=$(dirname "$trail_path")
	if ! mkdir -p "$trail_dir" 2>/dev/null; then
		echo "$PROG: warning: cannot create trail dir $trail_dir" >&2
		return 1
	fi
	# Mirror pkg/vroom/decisiontrail.OpenJSONL: create or tighten the shared
	# trail with owner-only permissions. touch is deliberately non-truncating:
	# another VROOM writer may append between our mkdir and this creation step.
	(umask 0177 && touch "$trail_path" && chmod 600 "$trail_path") 2>/dev/null || true
	event_id=$(new_event_id)
	ts=$(now_utc)
	esc_dir=$(json_escape "$gobin_dir")
	esc_sentinel=$(json_escape "$sentinel_path")
	esc_reason=$(json_escape "$reason")
	esc_status=$(json_escape "$status")
	esc_role=$(json_escape "$role")
	if ! printf '{"event_id":"%s","timestamp":"%s","role":"%s","kind":"watchdog.gobin.missing","payload":{"status":"%s","gobin_dir":"%s","sentinel":"%s","reason":"%s"}}\n' \
		"$event_id" "$ts" "$esc_role" "$esc_status" "$esc_dir" "$esc_sentinel" "$esc_reason" \
		>>"$trail_path" 2>/dev/null; then
		echo "$PROG: warning: trail append failed: $trail_path" >&2
		return 1
	fi
	return 0
}

notify_operator() {
	# The launchd agent runs on macOS. A durable trail is useful for audit, but
	# a notification is the active observation loop: the owner sees a GOBIN wipe
	# without waiting for another VROOM process or a manual log inspection.
	[ "${GOBIN_GUARD_NOTIFY:-1}" = "1" ] || return 1
	if [ "$(uname -s)" = "Darwin" ] && command -v osascript >/dev/null 2>&1; then
		if ! osascript -e 'display notification "The Go toolchain binary directory needs repair. See gobin-guard.err.log." with title "DEAR Agent GOBIN alarm"' >/dev/null 2>&1; then
			echo "$PROG: warning: macOS notification failed" >&2
			return 1
		fi
		return 0
	fi
	return 1
}

if [ "$status" = "ok" ]; then
	rm -f "$alarm_path" 2>/dev/null || true
	if [ "$json_output" -eq 1 ]; then
		printf '{"status":"ok","gobin_dir":"%s","sentinel":"%s"}\n' \
			"$(json_escape "$gobin_dir")" "$(json_escape "$sentinel_path")"
	elif [ "$quiet" -eq 0 ]; then
		echo "$PROG: OK ($gobin_dir, $sentinel_name executable)"
	fi
	exit 0
fi

# Degraded: report only the healthy-to-degraded transition. A missing GOBIN
# can persist for hours; repeating notifications and trail entries each tick
# obscures the original event rather than improving observability.
if ! is_alarm_marker "$alarm_path"; then
	delivered=1
	escalate && delivered=0
	notify_operator && delivered=0
	if [ "$delivered" -eq 0 ]; then
		persist_alarm_marker "$alarm_path" 2>/dev/null || \
			echo "$PROG: warning: cannot persist alarm state: $alarm_path" >&2
	fi
fi

if [ "$json_output" -eq 1 ]; then
	printf '{"status":"%s","gobin_dir":"%s","sentinel":"%s","reason":"%s"}\n' \
		"$(json_escape "$status")" "$(json_escape "$gobin_dir")" \
		"$(json_escape "$sentinel_path")" "$(json_escape "$reason")"
else
	echo "$PROG: ALARM ($status): $reason" >&2
fi
exit 1
