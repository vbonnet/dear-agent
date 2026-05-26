#!/usr/bin/env bash
# test-affected.sh — run only the Go tests affected by the current diff.
#
# Wraps cmd/test-affected so callers don't have to remember the pipeline.
# By default diffs against origin/main, scopes to the `integration` build
# tag, and runs `go test -race -count=1` on whatever the selector finds.
#
# Usage:
#   scripts/test-affected.sh                          # integration tests vs origin/main
#   scripts/test-affected.sh --base=origin/develop    # change the base
#   scripts/test-affected.sh --tags=integration,e2e   # union of tag-gated suites
#   scripts/test-affected.sh --tags=                  # no tags (unit-test scope)
#   scripts/test-affected.sh --dry-run                # print the package list, don't run
#
# Exit codes:
#   0  tests ran cleanly, or no tests were affected (this is a pass).
#   non-zero  the selected tests failed, or the selector errored.

set -euo pipefail

BASE="origin/main"
TAGS="integration"
DRY_RUN=0
EXTRA_TEST_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base=*)
      BASE="${1#*=}"
      shift
      ;;
    --base)
      BASE="$2"
      shift 2
      ;;
    --tags=*)
      TAGS="${1#*=}"
      shift
      ;;
    --tags)
      TAGS="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --)
      shift
      EXTRA_TEST_ARGS+=("$@")
      break
      ;;
    *)
      EXTRA_TEST_ARGS+=("$1")
      shift
      ;;
  esac
done

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

# Build flag list for the selector.
SELECTOR_ARGS=(--base="$BASE")
if [[ -n "$TAGS" ]]; then
  SELECTOR_ARGS+=(--tags="$TAGS")
fi

# Capture stderr separately so a failed selector doesn't get blended into
# the package list piped to `go test`.
PKG_LIST="$(mktemp)"
SELECTOR_ERR="$(mktemp)"
trap 'rm -f "$PKG_LIST" "$SELECTOR_ERR"' EXIT

if ! go run ./cmd/test-affected "${SELECTOR_ARGS[@]}" >"$PKG_LIST" 2>"$SELECTOR_ERR"; then
  cat "$SELECTOR_ERR" >&2
  echo "test-affected: selector failed; falling back to running all tests" >&2
  if [[ -n "$TAGS" ]]; then
    exec go test -tags="$TAGS" -race -count=1 "${EXTRA_TEST_ARGS[@]}" ./...
  fi
  exec go test -race -count=1 "${EXTRA_TEST_ARGS[@]}" ./...
fi

# Surface verbose-mode diagnostics if any were emitted.
if [[ -s "$SELECTOR_ERR" ]]; then
  cat "$SELECTOR_ERR" >&2
fi

# Empty output = no affected tests = clean pass.
if [[ ! -s "$PKG_LIST" ]]; then
  echo "test-affected: no test packages affected (base=$BASE tags=$TAGS)" >&2
  exit 0
fi

PKG_COUNT="$(wc -l <"$PKG_LIST" | tr -d ' ')"
echo "test-affected: running ${PKG_COUNT} package(s) (base=$BASE tags=$TAGS)" >&2

# mapfile is bash 4+, absent on macOS's stock bash. Read into PKGS the
# portable way; package import paths cannot contain whitespace, so a
# while-read loop with the default IFS is safe.
PKGS=()
while IFS= read -r line; do
  [[ -n "$line" ]] && PKGS+=("$line")
done <"$PKG_LIST"

if [[ "$DRY_RUN" -eq 1 ]]; then
  printf '%s\n' "${PKGS[@]}"
  exit 0
fi

CMD=(go test -race -count=1)
if [[ -n "$TAGS" ]]; then
  CMD+=(-tags="$TAGS")
fi
if [[ ${#EXTRA_TEST_ARGS[@]} -gt 0 ]]; then
  CMD+=("${EXTRA_TEST_ARGS[@]}")
fi
CMD+=("${PKGS[@]}")

exec "${CMD[@]}"
