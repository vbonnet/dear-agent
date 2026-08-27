#!/usr/bin/env bash
set -euo pipefail
scan_root="${1:-.}"
capture_dir=$(mktemp -d "${TMPDIR:-/tmp}/monthly-audit-complexity.XXXXXX")
trap 'rm -rf "$capture_dir"' EXIT
set +e
gocognit -over 15 "$scan_root" >"$capture_dir/stdout" 2>"$capture_dir/stderr"
status=$?
set -e
if [ "$status" -eq 0 ] && [ ! -s "$capture_dir/stdout" ] && [ ! -s "$capture_dir/stderr" ]; then
  exit 0
fi
if [ "$status" -eq 1 ] && [ -s "$capture_dir/stdout" ] && [ ! -s "$capture_dir/stderr" ]; then
  sed -n '1,50p' "$capture_dir/stdout"
  exit 0
fi
if [ "$status" -eq 0 ]; then status=2; fi
cat "$capture_dir/stderr" >&2
printf 'monthly cognitive-complexity scan failed (status=%s)\n' "$status" >&2
exit "$status"
