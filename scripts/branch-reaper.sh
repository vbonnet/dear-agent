#!/usr/bin/env bash
# branch-reaper.sh — periodic merged-branch cleanup + abandoned-branch visibility.
#
# WHY THIS EXISTS
#   GitHub's repo-level "automatically delete head branches" setting is ON for this
#   repo and correctly auto-deletes ~98.6% of merged PR branches. But it silently
#   fails on a small, unpredictable fraction of merges (no common pattern found in
#   merge method, actor, or PR stacking) -- 14 of 1032 merges over the repo's
#   history left their branch behind. Left unchecked these accumulate forever,
#   because nothing ever goes back and retries the delete. Separately, automated
#   branch-creating tooling (spec-coverage generators, wayfinder, codex goal
#   branches, review/audit bots) sometimes never opens a PR at all, leaving
#   abandoned work branches that neither this script nor GitHub's own mechanism
#   will ever clean up on their own. This script is the safety net for both.
#
# WHAT IT DOES
#   1. SAFE-DELETE (auto, on --execute): a branch whose most-recent-PR is MERGED
#      and whose branch-tip commit is NOT newer than that PR's mergedAt timestamp.
#      That means nothing was ever pushed to the branch after it merged, so the
#      branch's entire content is byte-for-byte what's already in main's history
#      via the squash commit. Zero risk of losing work.
#   2. REVIEW (report only, never deleted): branches with no PR at all, PRs that
#      were closed without merging, or "merged" branches that got new commits
#      pushed after the merge (real unmerged work). These need a human judgment
#      call, so this script only ever lists them -- it never deletes them.
#
# USAGE
#   scripts/branch-reaper.sh                 # dry run: report only, delete nothing
#   scripts/branch-reaper.sh --execute        # actually delete the SAFE-DELETE set
#   scripts/branch-reaper.sh --json           # machine-readable report on stdout
#
# REQUIRES: git (run inside a checkout with full history, e.g. actions/checkout
#           with fetch-depth: 0), gh (authenticated).
#
# EXIT CODES: 0 = ran clean · 1 = review-list non-empty (visibility signal, not a
#             failure) · 3 = usage/environment error.

set -u

EXECUTE=0
JSON_ONLY=0
REPO="${GH_REPO:-$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)}"
PROTECT_PATTERN='^(main|master|HEAD)$'
IGNORE_PREFIX_PATTERN='^dependabot/'   # dependabot manages its own branch lifecycle

while [ $# -gt 0 ]; do
  case "$1" in
    --execute) EXECUTE=1 ;;
    --json) JSON_ONLY=1 ;;
    -h|--help) sed -n '2,32p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 3 ;;
  esac
  shift
done

if [ -z "$REPO" ]; then
  echo "branch-reaper: could not determine repo (set GH_REPO or run inside a repo checkout)" >&2
  exit 3
fi
export GIT_TERMINAL_PROMPT=0

# ----- gather remote branches with their tip commit date, via git (not the API --
# the branches API's default listing does NOT include committer date, which is
# why the older stale-branch-audit workflow always saw an empty date and silently
# matched nothing; git for-each-ref gets it for free from the local checkout). -----
branches_raw="$(git for-each-ref --format='%(refname)|%(committerdate:iso-strict)' refs/remotes/origin \
  | grep -v '^refs/remotes/origin/HEAD|' \
  | sed 's#^refs/remotes/origin/##')"

safe_delete=()
review_no_pr=()
review_closed_unmerged=()
review_new_commits_after_merge=()
review_open_pr=()

while IFS='|' read -r branch tipdate; do
  [ -z "$branch" ] && continue
  echo "$branch" | grep -Eq "$PROTECT_PATTERN" && continue
  echo "$branch" | grep -Eq "$IGNORE_PREFIX_PATTERN" && continue

  prs_json="$(gh pr list --repo "$REPO" --head "$branch" --state all \
    --json number,state,mergedAt,createdAt --limit 20 2>/dev/null)"
  [ -z "$prs_json" ] && prs_json="[]"

  open_count="$(echo "$prs_json" | jq '[.[] | select(.state=="OPEN")] | length')"
  if [ "$open_count" -gt 0 ]; then
    review_open_pr+=("$branch")
    continue
  fi

  merged_at="$(echo "$prs_json" | jq -r '[.[] | select(.state=="MERGED")] | sort_by(.mergedAt) | last | .mergedAt // empty')"
  if [ -n "$merged_at" ]; then
    tip_epoch="$(date -d "$tipdate" +%s 2>/dev/null || date -jf '%Y-%m-%dT%H:%M:%S' "${tipdate%%[+-]*}" +%s 2>/dev/null)"
    merged_epoch="$(date -d "$merged_at" +%s 2>/dev/null || date -jf '%Y-%m-%dT%H:%M:%SZ' "$merged_at" +%s 2>/dev/null)"
    if [ -n "$tip_epoch" ] && [ -n "$merged_epoch" ] && [ "$tip_epoch" -le "$merged_epoch" ]; then
      safe_delete+=("$branch")
    else
      review_new_commits_after_merge+=("$branch")
    fi
    continue
  fi

  closed_count="$(echo "$prs_json" | jq '[.[] | select(.state=="CLOSED")] | length')"
  if [ "$closed_count" -gt 0 ]; then
    review_closed_unmerged+=("$branch")
  else
    review_no_pr+=("$branch")
  fi
done <<< "$branches_raw"

# ----- report ---------------------------------------------------------------
if [ "$JSON_ONLY" -eq 1 ]; then
  jq -n \
    --argjson safe "$(printf '%s\n' "${safe_delete[@]:-}" | jq -R . | jq -s 'map(select(length>0))')" \
    --argjson no_pr "$(printf '%s\n' "${review_no_pr[@]:-}" | jq -R . | jq -s 'map(select(length>0))')" \
    --argjson closed "$(printf '%s\n' "${review_closed_unmerged[@]:-}" | jq -R . | jq -s 'map(select(length>0))')" \
    --argjson new_commits "$(printf '%s\n' "${review_new_commits_after_merge[@]:-}" | jq -R . | jq -s 'map(select(length>0))')" \
    '{safe_delete: $safe, review_no_pr: $no_pr, review_closed_unmerged: $closed, review_new_commits_after_merge: $new_commits}'
else
  echo "## Branch reaper — $(date -u '+%Y-%m-%d')"
  echo
  echo "Safe to delete (merged, nothing pushed since): ${#safe_delete[@]}"
  printf '  - %s\n' "${safe_delete[@]:-}"
  echo
  echo "Review: no PR ever opened: ${#review_no_pr[@]}"
  printf '  - %s\n' "${review_no_pr[@]:-}"
  echo
  echo "Review: PR closed without merging: ${#review_closed_unmerged[@]}"
  printf '  - %s\n' "${review_closed_unmerged[@]:-}"
  echo
  echo "Review: merged, but new commits pushed after: ${#review_new_commits_after_merge[@]}"
  printf '  - %s\n' "${review_new_commits_after_merge[@]:-}"
fi

if [ "$EXECUTE" -eq 1 ]; then
  for b in "${safe_delete[@]:-}"; do
    [ -z "$b" ] && continue
    echo "deleting: $b" >&2
    git push origin --delete "$b"
  done
fi

review_total=$(( ${#review_no_pr[@]} + ${#review_closed_unmerged[@]} + ${#review_new_commits_after_merge[@]} ))
[ "$review_total" -gt 0 ] && exit 1
exit 0
