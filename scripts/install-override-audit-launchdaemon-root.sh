#!/bin/sh
set -eu
probe_exit=42; mode=
IFS= read -r mode || exit 2
test "$mode" != "PROBE" || exit "$probe_exit"
test "$mode" = "INSTALL" && test "$#" = 7 || exit 2
helper_artifact=$1; expected_helper_hash=$2; root_gid=$3; staging=
cleanup() { status=$1; trap - EXIT HUP INT TERM; set +e; /bin/rm -f "$staging"; exit "$status"; }
trap 'cleanup "$?"' EXIT
trap 'cleanup 129' HUP
trap 'cleanup 130' INT
trap 'cleanup 143' TERM
/usr/bin/install -d -o root -g "$root_gid" -m 0755 /usr/local/libexec
staging=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-override-audit-launchdaemon-installer.XXXXXX)
/usr/bin/install -o root -g "$root_gid" -m 0755 "$helper_artifact" "$staging"
staged_hash=$(/usr/bin/openssl dgst -sha256 -r "$staging"); staged_hash=${staged_hash%% *}; test "$staged_hash" = "$expected_helper_hash"
"$staging" "$root_gid" "$4" "$5" "$6" "$7"
/bin/rm -f "$staging"
staging=
trap - EXIT HUP INT TERM
