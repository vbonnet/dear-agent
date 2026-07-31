#!/bin/sh
set -eu
umask 077

probe_exit=42
mode=
IFS= read -r mode || exit 2
if test "$mode" = "PROBE"; then
	exit "$probe_exit"
fi
test "$mode" = "INSTALL" || exit 2
test "$#" = 5 || exit 2

root_gid=$1
audit_artifact=$2
plist_artifact=$3
expected_audit_hash=$4
expected_plist_hash=$5

case "$root_gid" in
	*[!0-9]*|"") exit 2 ;;
esac
for artifact in "$audit_artifact" "$plist_artifact"; do
	case "$artifact" in
		/*) ;;
		*) exit 2 ;;
	esac
	test -f "$artifact"
done
for expected_hash in "$expected_audit_hash" "$expected_plist_hash"; do
	case "$expected_hash" in
		*[!0-9a-f]*|"") exit 2 ;;
	esac
	test "${#expected_hash}" -eq 64
done

audit_live=/usr/local/libexec/dear-agent-override-audit
plist_live=/Library/LaunchDaemons/com.dear-agent.override-audit.plist
audit_staging=
plist_staging=
audit_backup=
plist_backup=
audit_existed=0
plist_existed=0
activation_started=0
activation_complete=0

cleanup_launchdaemon_staging() {
	status=$1
	trap - EXIT HUP INT TERM
	set +e
	cleanup_failed=0
	if test "$activation_started" = 1 && test "$activation_complete" != 1; then
		if test "$audit_existed" = 1; then
			if /bin/mv -f "$audit_backup" "$audit_live"; then
				audit_backup=
			else
				cleanup_failed=1
			fi
		else
			/bin/rm -f "$audit_live" || cleanup_failed=1
		fi
		if test "$plist_existed" = 1; then
			if /bin/mv -f "$plist_backup" "$plist_live"; then
				plist_backup=
			else
				cleanup_failed=1
			fi
		else
			/bin/rm -f "$plist_live" || cleanup_failed=1
		fi
	fi
	/bin/rm -f "$audit_staging" "$plist_staging" || cleanup_failed=1
	if test "$activation_started" != 1 || test "$activation_complete" = 1; then
		/bin/rm -f "$audit_backup" "$plist_backup" || cleanup_failed=1
	fi
	if test "$cleanup_failed" = 1; then
		echo "LaunchDaemon audit installer cleanup or rollback was incomplete" >&2
		if test "$status" -eq 0; then
			status=1
		fi
	fi
	exit "$status"
}

trap 'cleanup_launchdaemon_staging "$?"' EXIT
trap 'cleanup_launchdaemon_staging 129' HUP
trap 'cleanup_launchdaemon_staging 130' INT
trap 'cleanup_launchdaemon_staging 143' TERM

/usr/bin/install -d -o root -g "$root_gid" -m 0755 /usr/local/libexec
audit_staging=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit.XXXXXX)
plist_staging=$(/usr/bin/mktemp /Library/LaunchDaemons/.com.dear-agent.override-audit.XXXXXX)
/usr/bin/install -o root -g "$root_gid" -m 0755 "$audit_artifact" "$audit_staging"
/usr/bin/install -o root -g "$root_gid" -m 0644 "$plist_artifact" "$plist_staging"

staged_audit_hash=$(/usr/bin/openssl dgst -sha256 -r "$audit_staging")
staged_audit_hash=${staged_audit_hash%% *}
staged_plist_hash=$(/usr/bin/openssl dgst -sha256 -r "$plist_staging")
staged_plist_hash=${staged_plist_hash%% *}
test "$staged_audit_hash" = "$expected_audit_hash"
test "$staged_plist_hash" = "$expected_plist_hash"
/usr/bin/plutil -lint "$plist_staging" >/dev/null

if test -e "$audit_live"; then
	audit_existed=1
	audit_backup=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit.backup.XXXXXX)
	/bin/cp -p "$audit_live" "$audit_backup"
fi
if test -e "$plist_live"; then
	plist_existed=1
	plist_backup=$(/usr/bin/mktemp /Library/LaunchDaemons/.com.dear-agent.override-audit.backup.XXXXXX)
	/bin/cp -p "$plist_live" "$plist_backup"
fi

activation_started=1
/bin/mv -f "$audit_staging" "$audit_live"
audit_staging=
/bin/mv -f "$plist_staging" "$plist_live"
plist_staging=
activation_complete=1
/bin/rm -f "$audit_backup" "$plist_backup"
audit_backup=
plist_backup=
trap - EXIT HUP INT TERM
