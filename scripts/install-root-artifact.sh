#!/bin/sh
set -eu
test "$#" = 4 && test "$0" = dear-agent-root-artifact-installer || exit 2
IFS= read -r mode; case "$mode" in PROBE|INSTALL) ;; *) exit 2;; esac
test "$mode" != PROBE || exit 42
test "$(/usr/bin/id -u)" = 0 || exit 2
artifact=$1; expected_hash=$2; root_gid=$3; destination=$4
case "$destination" in /usr/local/libexec/dear-agent-codex-hook-json|/usr/local/libexec/dear-agent-bead-close-guard) ;; *) exit 2;; esac
staging=
cleanup() { status=$?; trap - EXIT HUP INT TERM; test -z "$staging" || /bin/rm -f "$staging"; exit "$status"; }
trap 'cleanup' EXIT HUP INT TERM
platform=$(/usr/bin/uname -s); case "$platform" in Darwin) trusted_parent=/private/var/root; trusted_chain="/private /private/var $trusted_parent";; Linux) trusted_parent=/root; trusted_chain="/ $trusted_parent";; *) exit 2;; esac
trusted() { test -d "$1" && test ! -L "$1" || return 1; case "$platform" in Darwin) uid=$(/usr/bin/stat -f '%u' "$1"); mode_bits=$(/usr/bin/stat -f '%Lp' "$1");; Linux) uid=$(/usr/bin/stat -c '%u' "$1"); mode_bits=$(/usr/bin/stat -c '%a' "$1");; esac; test "$uid" = 0 && test "$((0$mode_bits & 0022))" -eq 0; }
for dir in $trusted_chain; do trusted "$dir" || exit 2; done
trusted_dir=$trusted_parent/.dear-agent-root-artifact; /usr/bin/install -d -o root -g "$root_gid" -m 0700 "$trusted_dir"
staging=$(/usr/bin/mktemp "$trusted_dir/.artifact.XXXXXX"); /usr/bin/install -o root -g "$root_gid" -m 0755 "$artifact" "$staging"
staged_hash=$(/usr/bin/openssl dgst -sha256 -r "$staging"); staged_hash=${staged_hash%% *}; test "$staged_hash" = "$expected_hash"
for dir in / /usr /usr/local; do trusted "$dir" || exit 2; done
if test -e /usr/local/libexec || test -L /usr/local/libexec; then trusted /usr/local/libexec || exit 2; else /usr/bin/install -d -o root -g "$root_gid" -m 0755 /usr/local/libexec; trusted /usr/local/libexec || exit 2; fi
/bin/mv -f "$staging" "$destination"; staging=; trap - EXIT HUP INT TERM
