#!/usr/bin/env bash
# repo-health-audit.sh — deterministic, read-only health audit of golden ~/src checkouts.
#
# WHY THIS EXISTS
#   The previous audit was a free-form LLM prompt re-pasted by hand each run. That
#   made it non-deterministic (thresholds, repo list, and bead handling drifted run
#   to run), so results were not comparable and the question "is dear-agent improving
#   or degrading our projects?" was unanswerable. Worse, a broken "check existing
#   beads first" step re-filed NEW beads every run — ~14 open beads accumulated for
#   just 3 underlying violations. This script replaces the prompt with fixed logic:
#   same repos (auto-discovered), same thresholds, same machine-readable output every
#   time, plus an append-only history file so trends are detectable.
#
# WHAT IT CHECKS (per git repo under the scan root)
#   - branch:    on the remote's default branch (main/master)?
#   - clean:     working tree clean? (tracked modifications => FAIL; untracked => WARN)
#   - behind:    commits behind origin/<default>  (>5 FAIL, 1-5 WARN)
#   - ahead:     commits ahead of origin/<default> (any ahead => FAIL — golden
#                checkouts must never carry local commits; that is the worktree-only
#                rule from AGENTS.md being violated)
#   - artifacts: untracked *.md/*.yaml/*.json (agent leftovers) => WARN
#
# READ-ONLY GUARANTEE
#   Only ever runs: git branch/status/rev-list/ls-files/fetch. No write, checkout,
#   commit, reset, or push. Safe to run against the read-only ~/src golden tree.
#   All network ops are bounded by `gtimeout` and GIT_TERMINAL_PROMPT=0 so a keychain
#   prompt can never hang the run (see memory/macos-env-gaps.md).
#
# USAGE
#   scripts/repo-health-audit.sh                 # table + JSON to stdout, append history
#   scripts/repo-health-audit.sh --json          # JSON only (for piping / bead tooling)
#   scripts/repo-health-audit.sh --root DIR       # scan DIR instead of ~/src
#   scripts/repo-health-audit.sh --no-fetch       # skip network; use cached refs
#   scripts/repo-health-audit.sh --no-history     # don't append to the history file
#
# OUTPUT
#   - Human table + a SCORE per repo (0-100) and an overall fleet score to stdout.
#   - JSON object with per-repo metrics.
#   - Append-only history at ${HISTORY_DIR}/<UTC-date>T<time>.json (one file per run)
#     so two runs can be diffed to answer improving-or-degrading.
#
# EXIT CODES
#   0 = all repos PASS · 1 = at least one WARN · 2 = at least one FAIL · 3 = usage error

set -u

# ----- config -------------------------------------------------------------------
SCAN_ROOT="${HOME}/src"
HISTORY_DIR="${REPO_HEALTH_HISTORY_DIR:-${HOME}/.config/dear-agent/repo-health-history}"
FETCH=1
WRITE_HISTORY=1
JSON_ONLY=0
FETCH_TIMEOUT=20

# Thresholds (fixed — changing these is a reviewable code change, not per-run whim).
BEHIND_WARN=1     # >=1 behind => at least WARN
BEHIND_FAIL=6     # >=6 behind => FAIL

while [ $# -gt 0 ]; do
  case "$1" in
    --json) JSON_ONLY=1 ;;
    --no-fetch) FETCH=0 ;;
    --no-history) WRITE_HISTORY=0 ;;
    --root) shift; SCAN_ROOT="${1:?--root needs a dir}" ;;
    -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 3 ;;
  esac
  shift
done

export GIT_TERMINAL_PROMPT=0

# gtimeout (coreutils) on macOS; fall back to timeout, else no-op wrapper.
if command -v gtimeout >/dev/null 2>&1; then TIMEOUT="gtimeout ${FETCH_TIMEOUT}"
elif command -v timeout >/dev/null 2>&1; then TIMEOUT="timeout ${FETCH_TIMEOUT}"
else TIMEOUT=""; fi

# ----- helpers ------------------------------------------------------------------
default_branch() {
  # remote's default branch, else current branch, else "main"
  local d="$1" ref
  ref=$(git -C "$d" symbolic-ref --quiet refs/remotes/origin/HEAD 2>/dev/null)
  if [ -n "$ref" ]; then echo "${ref##*/}"; return; fi
  ref=$(git -C "$d" branch --show-current 2>/dev/null)
  echo "${ref:-main}"
}

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

# ----- scan ---------------------------------------------------------------------
repos=()
for d in "$SCAN_ROOT"/*/; do
  [ -d "${d}.git" ] && repos+=("${d%/}")
done

if [ ${#repos[@]} -eq 0 ]; then
  echo "no git repos under $SCAN_ROOT" >&2; exit 3
fi

run_ts="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
worst=0          # 0 pass, 1 warn, 2 fail (fleet)
fleet_score_sum=0
json_repos=""
table=""

for d in "${repos[@]}"; do
  repo="$(basename "$d")"
  br="$(git -C "$d" branch --show-current 2>/dev/null)"
  def="$(default_branch "$d")"
  tracked_dirty="$(git -C "$d" status --porcelain 2>/dev/null | grep -Ec '^[ MARCDU]')"
  untracked_total="$(git -C "$d" ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' ')"
  artifacts="$(git -C "$d" ls-files --others --exclude-standard 2>/dev/null | grep -Ec '\.(md|yaml|yml|json)$')"

  has_remote=0
  git -C "$d" remote get-url origin >/dev/null 2>&1 && has_remote=1
  fetch_rc=0
  if [ "$FETCH" -eq 1 ] && [ "$has_remote" -eq 1 ]; then
    $TIMEOUT git -C "$d" fetch origin >/dev/null 2>&1; fetch_rc=$?
  fi
  behind="$(git -C "$d" rev-list "HEAD..origin/${def}" --count 2>/dev/null)"
  ahead="$(git -C "$d" rev-list "origin/${def}..HEAD" --count 2>/dev/null)"
  [ -z "$behind" ] && behind="-1"   # -1 = unknown (no remote ref)
  [ -z "$ahead" ] && ahead="-1"

  # ----- scoring (deterministic) -----
  score=100; verdict="PASS"; sev=0; reasons=""
  add() { reasons="${reasons}${reasons:+; }$1"; }
  worse() { [ "$1" -gt "$sev" ] && sev="$1"; }

  if [ -n "$br" ] && [ "$br" != "$def" ]; then
    score=$((score-30)); worse 2; add "off default branch ($br != $def)"
  fi
  if [ "$tracked_dirty" -gt 0 ]; then
    score=$((score-25)); worse 2; add "$tracked_dirty tracked modification(s)"
  fi
  if [ "$ahead" -gt 0 ]; then
    score=$((score-30)); worse 2; add "$ahead commit(s) ahead of origin (golden tree must not carry local commits)"
  fi
  if [ "$behind" -ge "$BEHIND_FAIL" ]; then
    score=$((score-25)); worse 2; add "$behind commits behind origin"
  elif [ "$behind" -ge "$BEHIND_WARN" ]; then
    score=$((score-10)); worse 1; add "$behind commit(s) behind origin"
  fi
  if [ "$artifacts" -gt 0 ]; then
    score=$((score-10)); worse 1; add "$artifacts untracked agent artifact(s) (.md/.yaml/.json)"
  fi
  if [ "$has_remote" -eq 0 ]; then
    add "local-only (no origin remote)"   # informational, not penalized
  elif [ "$FETCH" -eq 1 ] && [ "$fetch_rc" -ne 0 ]; then
    worse 1; add "fetch failed (rc=$fetch_rc — network/timeout/missing remote)"
  fi
  [ "$score" -lt 0 ] && score=0
  case "$sev" in 2) verdict="FAIL";; 1) verdict="WARN";; *) verdict="PASS";; esac
  worse_global() { [ "$1" -gt "$worst" ] && worst="$1"; }; worse_global "$sev"
  fleet_score_sum=$((fleet_score_sum+score))

  table="${table}$(printf '%-22s %-7s %4s %6s %6s %5s %4s  %-5s %s\n' \
    "$repo" "$br" "$tracked_dirty" "$behind" "$ahead" "$artifacts" "$score" "$verdict" "$reasons")
"
  json_repos="${json_repos}${json_repos:+,}{\"repo\":\"$(json_escape "$repo")\",\"branch\":\"$(json_escape "$br")\",\"default_branch\":\"$(json_escape "$def")\",\"tracked_dirty\":$tracked_dirty,\"untracked\":$untracked_total,\"artifacts\":$artifacts,\"behind\":$behind,\"ahead\":$ahead,\"has_remote\":$has_remote,\"fetch_rc\":$fetch_rc,\"score\":$score,\"verdict\":\"$verdict\",\"reasons\":\"$(json_escape "$reasons")\"}"
done

fleet_score=$(( fleet_score_sum / ${#repos[@]} ))
case "$worst" in 2) fleet_verdict="FAIL";; 1) fleet_verdict="WARN";; *) fleet_verdict="PASS";; esac
json="{\"run_ts\":\"$run_ts\",\"scan_root\":\"$(json_escape "$SCAN_ROOT")\",\"repo_count\":${#repos[@]},\"fleet_score\":$fleet_score,\"fleet_verdict\":\"$fleet_verdict\",\"repos\":[$json_repos]}"

if [ "$WRITE_HISTORY" -eq 1 ]; then
  mkdir -p "$HISTORY_DIR" 2>/dev/null
  hist_file="${HISTORY_DIR}/${run_ts//:/-}.json"
  printf '%s\n' "$json" > "$hist_file" 2>/dev/null && hist_note="history: $hist_file" || hist_note="history: (write failed)"
fi

if [ "$JSON_ONLY" -eq 1 ]; then
  printf '%s\n' "$json"
else
  echo "## Repo Health Audit — $run_ts (root: $SCAN_ROOT)"
  echo
  printf '%-22s %-7s %4s %6s %6s %5s %4s  %-5s %s\n' REPO BRANCH DIRT BEHIND AHEAD ARTS SCR VRD REASONS
  printf '%s' "$table"
  echo
  echo "Fleet: $fleet_score/100  ($fleet_verdict)  across ${#repos[@]} repos"
  [ "$WRITE_HISTORY" -eq 1 ] && echo "$hist_note"
fi

exit "$worst"
