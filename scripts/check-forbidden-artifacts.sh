#!/usr/bin/env bash
#
# check-forbidden-artifacts.sh — block temporal artifacts from the code repo.
#
# You're trying to commit a file to dear-agent. This guard exists because
# *temporal* artifacts (Wayfinder SDLC runs — W0/D1-D4/S4-S11,
# WAYFINDER-STATUS.md, WAYFINDER-HISTORY.md, .wayfinder/ runs — plus retros,
# designs, and research) must NOT live in this code repo. They capture a
# moment of thinking, are not maintained as the code evolves, and belong in
# the knowledge base (~/src/engram-research, conventionally wf/<project>/).
# The Wayfinder TOOL SOURCE (wayfinder/, *.go, validator testdata, ADR-031)
# is living code/docs and stays here — it is deliberately not matched.
#
# Source of truth for the forbidden globs is .dear-agent.yml > forbidden-paths.
# This script PARSES that file at runtime so the rule can never drift from a
# hand-copied list (the root cause of the 2026-06-19 Wayfinder leak — see the
# DEAR retro in engram-research). One list, three call sites: pre-commit
# (--staged), CI diff (--diff), CI whole-tree (--all).
#
# Usage:
#   check-forbidden-artifacts.sh --all              # scan every tracked file
#   check-forbidden-artifacts.sh --staged           # scan staged (pre-commit)
#   check-forbidden-artifacts.sh --diff <base-ref>  # scan files added vs base
#   check-forbidden-artifacts.sh --files -          # read paths from stdin
#
# Add  --baseline <file>  to exempt a documented set of pre-existing,
# grandfathered violations (one path per line, '#' comments allowed). The
# baseline is shrink-only: a new violation NOT in the baseline still fails,
# so existing debt is frozen and visible without blocking unrelated work.
#
# Exit 0 = clean, 1 = violations found, 2 = usage/internal error.

# Note: no `set -u` — bash 3.2 (macOS) errors on "${empty_array[@]}" under
# nounset, and this script intentionally works with possibly-empty arrays.
set -eo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
YML="${REPO_ROOT}/.dear-agent.yml"

if [[ ! -f "$YML" ]]; then
  echo "check-forbidden-artifacts: no .dear-agent.yml at repo root; nothing to enforce" >&2
  exit 0
fi

# --- extract forbidden globs from .dear-agent.yml > forbidden-paths block ----
# Collect every "- <glob>" list item under the forbidden-paths: mapping,
# stripping surrounding quotes and trailing comments. The block ends at the
# next top-level (column-0) key.
# (read loop instead of mapfile/readarray — macOS ships bash 3.2)
PATTERNS=()
while IFS= read -r _line; do
  [[ -n "$_line" ]] && PATTERNS+=("$_line")
done < <(
  awk '
    /^forbidden-paths:/ { inblock=1; next }
    inblock && /^[^[:space:]#]/ { inblock=0 }
    inblock && /^[[:space:]]*-[[:space:]]/ {
      line = $0
      sub(/^[[:space:]]*-[[:space:]]*/, "", line)   # drop "  - "
      sub(/[[:space:]]+#.*$/, "", line)             # drop trailing comment
      gsub(/^["'"'"']|["'"'"']$/, "", line)         # drop surrounding quotes
      gsub(/[[:space:]]+$/, "", line)
      if (line != "") print line
    }
  ' "$YML"
)

if [[ ${#PATTERNS[@]} -eq 0 ]]; then
  echo "check-forbidden-artifacts: forbidden-paths is empty in .dear-agent.yml" >&2
  exit 0
fi

# --- match one repo-relative path against one glob --------------------------
# bash 3.2 (macOS) quirk: in [[ str == pat ]] a '*' reliably spans '/' only at
# the END of the pattern, not mid-pattern after a literal '/'. So we classify
# each forbidden glob into a structural shape and match with an end-anchored
# '*' (or a substring/basename test), never a mid-pattern slash-spanning '*'.
path_is_forbidden() {
  local f="$1" pat base="${1##*/}" pre mid suf
  for pat in "${PATTERNS[@]}"; do
    case "$pat" in
      '**/'*'/**')                 # **/mid/**  -> path contains /mid/ segment
        mid="${pat#\*\*/}"; mid="${mid%/\*\*}"
        [[ "/$f" == *"/$mid/"* ]] && return 0
        ;;
      *'/**')                      # prefix/**  -> file under directory prefix
        pre="${pat%/\*\*}"
        [[ "$f" == "$pre" || "$f" == "$pre/"* ]] && return 0
        ;;
      '**/'*)                      # **/NAME    -> basename glob (any depth)
        suf="${pat#\*\*/}"
        # shellcheck disable=SC2053
        [[ "$base" == $suf ]] && return 0
        ;;
      *)                           # plain relative glob, e.g. research/*.md
        # shellcheck disable=SC2053
        [[ "$f" == $pat ]] && return 0
        ;;
    esac
  done
  return 1
}

# --- parse args: a mode (+ optional operand) and an optional --baseline ------
mode=""
mode_operand=""
BASELINE_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --baseline) BASELINE_FILE="${2:?--baseline requires a file}"; shift 2 ;;
    --all|--staged) mode="$1"; shift ;;
    --diff|--files) mode="$1"; mode_operand="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,34p' "$0"; exit 0 ;;
    *) echo "check-forbidden-artifacts: unknown arg '$1' (see --help)" >&2; exit 2 ;;
  esac
done
[[ -z "$mode" ]] && mode="--all"

# Load baseline exemptions (repo-relative paths) into an associative-free set.
BASELINE=()
if [[ -n "$BASELINE_FILE" ]]; then
  [[ -f "$BASELINE_FILE" ]] || { echo "check-forbidden-artifacts: baseline not found: $BASELINE_FILE" >&2; exit 2; }
  while IFS= read -r _b; do
    _b="${_b%%#*}"; _b="${_b#"${_b%%[![:space:]]*}"}"; _b="${_b%"${_b##*[![:space:]]}"}"
    [[ -n "$_b" ]] && BASELINE+=("$_b")
  done < "$BASELINE_FILE"
fi
is_baselined() {
  local p="$1" b
  for b in "${BASELINE[@]}"; do [[ "$p" == "$b" ]] && return 0; done
  return 1
}

# --- gather candidate paths by mode -----------------------------------------
FILES=()
_collect() { while IFS= read -r _f; do FILES+=("$_f"); done; }
case "$mode" in
  --all)
    _collect < <(git -C "$REPO_ROOT" ls-files)
    ;;
  --staged)
    _collect < <(git -C "$REPO_ROOT" diff --cached --name-only --diff-filter=ACMR)
    ;;
  --diff)
    base="${mode_operand:?--diff requires a base ref}"
    _collect < <(git -C "$REPO_ROOT" diff --name-only --diff-filter=ACMR "${base}...HEAD")
    ;;
  --files)
    src="${mode_operand:--}"
    if [[ "$src" == "-" ]]; then _collect; else _collect < "$src"; fi
    ;;
esac

# --- evaluate ----------------------------------------------------------------
declare -a VIOLATIONS=()
for f in "${FILES[@]}"; do
  [[ -z "$f" ]] && continue
  if path_is_forbidden "$f"; then
    is_baselined "$f" && continue   # grandfathered, documented pre-existing debt
    VIOLATIONS+=("$f")
  fi
done

if [[ ${#VIOLATIONS[@]} -eq 0 ]]; then
  exit 0
fi

{
  echo ""
  echo "ROUTING VIOLATION — temporal artifacts must not live in this code repo."
  echo ""
  echo "Forbidden files detected (${#VIOLATIONS[@]}):"
  printf '  - %s\n' "${VIOLATIONS[@]}"
  echo ""
  echo "These belong in the knowledge base instead:"
  echo "  Wayfinder runs / retros / designs  ->  ~/src/engram-research/wf/<project>/"
  echo ""
  echo "How to fix:"
  echo "  1. git rm --cached the file(s) and remove them from this repo."
  echo "  2. Move the content to engram-research (open a PR there)."
  echo "  3. Re-commit here without the artifact."
  echo ""
  echo "Why: temporal artifacts rot in code repos — they go stale silently,"
  echo "clutter blame history, and strand the work away from the corpus. The"
  echo "forbidden globs are defined in .dear-agent.yml > forbidden-paths."
  echo ""
} >&2
exit 1
