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
test "$#" = 7 || exit 2

root_group=$1
audit_artifact=$2
service_artifact=$3
timer_artifact=$4
expected_audit_hash=$5
expected_service_hash=$6
expected_timer_hash=$7

case "$root_group" in
	*[!A-Za-z0-9._-]*|"") exit 2 ;;
esac
for artifact in "$audit_artifact" "$service_artifact" "$timer_artifact"; do
	case "$artifact" in
		/*) ;;
		*) exit 2 ;;
	esac
	test -f "$artifact"
done
for expected_hash in "$expected_audit_hash" "$expected_service_hash" "$expected_timer_hash"; do
	case "$expected_hash" in
		*[!0-9a-f]*|"") exit 2 ;;
	esac
	test "${#expected_hash}" -eq 64
done

audit_staging=
service_staging=
timer_staging=
audit_live=/usr/local/libexec/dear-agent-override-audit
service_live=/etc/systemd/system/dear-agent-override-audit@.service
timer_live=/etc/systemd/system/dear-agent-override-audit@.timer
audit_backup=
service_backup=
timer_backup=
audit_existed=0
service_existed=0
timer_existed=0
activation_started=0
activation_complete=0

cleanup_systemd_staging() {
	status=$1
	trap - EXIT HUP INT TERM
	set +e
	rolled_back=0
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
		if test "$service_existed" = 1; then
			if /bin/mv -f "$service_backup" "$service_live"; then
				service_backup=
			else
				cleanup_failed=1
			fi
		else
			/bin/rm -f "$service_live" || cleanup_failed=1
		fi
		if test "$timer_existed" = 1; then
			if /bin/mv -f "$timer_backup" "$timer_live"; then
				timer_backup=
			else
				cleanup_failed=1
			fi
		else
			/bin/rm -f "$timer_live" || cleanup_failed=1
		fi
		rolled_back=1
	fi
	/bin/rm -f "$audit_staging" "$service_staging" "$timer_staging" ||
		cleanup_failed=1
	if test "$activation_started" != 1 || test "$activation_complete" = 1; then
		/bin/rm -f "$audit_backup" "$service_backup" "$timer_backup" ||
			cleanup_failed=1
	fi
	if test "$rolled_back" = 1; then
		/usr/bin/systemctl daemon-reload >/dev/null 2>&1 || cleanup_failed=1
	fi
	if test "$cleanup_failed" = 1; then
		echo "systemd audit installer cleanup or rollback was incomplete" >&2
		if test "$status" -eq 0; then
			status=1
		fi
	fi
	exit "$status"
}

trap 'cleanup_systemd_staging "$?"' EXIT
trap 'cleanup_systemd_staging 129' HUP
trap 'cleanup_systemd_staging 130' INT
trap 'cleanup_systemd_staging 143' TERM

/usr/bin/install -d -o root -g "$root_group" -m 0755 /usr/local/libexec
audit_staging=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit.XXXXXX)
service_staging=$(/usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-service.XXXXXX)
timer_staging=$(/usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-timer.XXXXXX)
/usr/bin/install -o root -g "$root_group" -m 0755 "$audit_artifact" "$audit_staging"
/usr/bin/install -o root -g "$root_group" -m 0644 "$service_artifact" "$service_staging"
/usr/bin/install -o root -g "$root_group" -m 0644 "$timer_artifact" "$timer_staging"

staged_audit_hash=$(/usr/bin/openssl dgst -sha256 -r "$audit_staging")
staged_audit_hash=${staged_audit_hash%% *}
staged_service_hash=$(/usr/bin/openssl dgst -sha256 -r "$service_staging")
staged_service_hash=${staged_service_hash%% *}
staged_timer_hash=$(/usr/bin/openssl dgst -sha256 -r "$timer_staging")
staged_timer_hash=${staged_timer_hash%% *}
test "$staged_audit_hash" = "$expected_audit_hash"
test "$staged_service_hash" = "$expected_service_hash"
test "$staged_timer_hash" = "$expected_timer_hash"

if test -e "$audit_live"; then
	audit_existed=1
	audit_backup=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit.backup.XXXXXX)
	/bin/cp -p "$audit_live" "$audit_backup"
fi
if test -e "$service_live"; then
	service_existed=1
	service_backup=$(/usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-service.backup.XXXXXX)
	/bin/cp -p "$service_live" "$service_backup"
fi
if test -e "$timer_live"; then
	timer_existed=1
	timer_backup=$(/usr/bin/mktemp /etc/systemd/system/.dear-agent-override-audit-timer.backup.XXXXXX)
	/bin/cp -p "$timer_live" "$timer_backup"
fi

activation_started=1
/bin/mv -f "$audit_staging" "$audit_live"
audit_staging=
/bin/mv -f "$service_staging" "$service_live"
service_staging=
/bin/mv -f "$timer_staging" "$timer_live"
timer_staging=
/usr/bin/systemctl daemon-reload
activation_complete=1
/bin/rm -f "$audit_backup" "$service_backup" "$timer_backup"
audit_backup=
service_backup=
timer_backup=
trap - EXIT HUP INT TERM
