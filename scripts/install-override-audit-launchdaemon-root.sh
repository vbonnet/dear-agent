#!/bin/sh
set -eu
probe_exit=42; mode=
IFS= read -r mode || exit 2
test "$mode" != "PROBE" || exit "$probe_exit"
test "$mode" = "INSTALL" && test "$#" = 7 || exit 2
helper_artifact=$1; expected_helper_hash=$2; root_gid=$3; staging=; child=
cleanup() { status=$1; trap - EXIT HUP INT TERM; set +e; /bin/rm -f "$staging"; exit "$status"; }
forward() { signal=$1; status=$2; trap - HUP INT TERM; test -z "$child" || { /bin/kill "-$signal" "$child" 2>/dev/null || :; wait "$child" || :; }; cleanup "$status"; }
trap 'cleanup "$?"' EXIT
trap 'forward HUP 129' HUP
trap 'forward INT 130' INT
trap 'forward TERM 143' TERM
trusted_parent=/private/var/root; for dir in /private /private/var "$trusted_parent"; do test -d "$dir" && test ! -L "$dir" && test "$(/usr/bin/stat -f '%u' "$dir")" = 0 && mode_bits=$(/usr/bin/stat -f '%Lp' "$dir") && test "$((0$mode_bits & 0022))" -eq 0 || exit 2; done; trusted_dir=$trusted_parent/.dear-agent-override-audit; /usr/bin/install -d -o root -g "$root_gid" -m 0700 "$trusted_dir"
staging=$(/usr/bin/mktemp "$trusted_dir/.launchdaemon-installer.XXXXXX")
/usr/bin/install -o root -g "$root_gid" -m 0755 "$helper_artifact" "$staging"
staged_hash=$(/usr/bin/openssl dgst -sha256 -r "$staging"); staged_hash=${staged_hash%% *}; test "$staged_hash" = "$expected_helper_hash"
"$staging" "$root_gid" "$4" "$5" "$6" "$7" & child=$!
wait "$child"
/bin/rm -f "$staging"; staging=; trap - EXIT HUP INT TERM
