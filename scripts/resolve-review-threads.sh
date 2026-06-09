#!/usr/bin/env bash
# resolve-review-threads.sh — list and resolve GitHub PR review threads.
#
# WHY THIS EXISTS
#   Gemini / bot reviewers open review threads on PRs. dear-agent's main branch
#   has required_conversation_resolution=true, so a PR cannot merge while any
#   thread is unresolved. Replying to a comment does NOT resolve its thread —
#   resolution is a distinct GraphQL mutation. This wrapper lets an agent
#   resolve bot threads without a human clicking "Resolve conversation".
#
# KEY FACT (verified against the live GitHub GraphQL schema 2026-06-09):
#   - Thread resolution lives ONLY in GraphQL. There is NO REST endpoint.
#   - The mutation is resolveReviewThread(input:{threadId: ID!}).
#     The input field is `threadId` (NOT `pullRequestReviewThreadId`).
#   - Thread IDs come from repository.pullRequest.reviewThreads[].id and look
#     like "PRRT_kwDO...". They are NOT the review-comment IDs from REST.
#
# USAGE
#   resolve-review-threads.sh list        <owner> <repo> <pr>           # unresolved threads
#   resolve-review-threads.sh list-all    <owner> <repo> <pr>           # every thread
#   resolve-review-threads.sh resolve     <threadId>                    # one thread by ID
#   resolve-review-threads.sh resolve-all <owner> <repo> <pr> [author]  # all unresolved
#                                                                       # (optional author filter,
#                                                                       #  e.g. gemini-code-assist)
#   resolve-review-threads.sh unresolve   <threadId>                    # re-open a thread
#
# Requires: gh (authenticated), jq.
set -euo pipefail

die() { echo "error: $*" >&2; exit 1; }
command -v gh >/dev/null || die "gh not found"
command -v jq >/dev/null || die "jq not found"

# Print review threads for a PR as compact JSON lines:
#   {id, isResolved, isOutdated, path, author, body}
_list_threads() {
  local owner=$1 repo=$2 pr=$3
  gh api graphql -f owner="$owner" -f repo="$repo" -F pr="$pr" -f query='
    query($owner:String!, $repo:String!, $pr:Int!) {
      repository(owner:$owner, name:$repo) {
        pullRequest(number:$pr) {
          reviewThreads(first:100) {
            totalCount
            nodes {
              id
              isResolved
              isOutdated
              path
              comments(first:1) { nodes { author { login } body } }
            }
          }
        }
      }
    }' --jq '
      .data.repository.pullRequest.reviewThreads.nodes[] | {
        id,
        isResolved,
        isOutdated,
        path,
        author: (.comments.nodes[0].author.login // "unknown"),
        body:   ((.comments.nodes[0].body // "") | gsub("\\s+"; " ") | .[0:120])
      }'
}

_resolve() {
  local thread=$1
  gh api graphql -f threadId="$thread" -f query='
    mutation($threadId:ID!) {
      resolveReviewThread(input:{threadId:$threadId}) {
        thread { id isResolved }
      }
    }' --jq '.data.resolveReviewThread.thread | "resolved \(.id) (isResolved=\(.isResolved))"'
}

_unresolve() {
  local thread=$1
  gh api graphql -f threadId="$thread" -f query='
    mutation($threadId:ID!) {
      unresolveReviewThread(input:{threadId:$threadId}) {
        thread { id isResolved }
      }
    }' --jq '.data.unresolveReviewThread.thread | "unresolved \(.id) (isResolved=\(.isResolved))"'
}

cmd=${1:-}; shift || true
case "$cmd" in
  list)
    [ $# -eq 3 ] || die "usage: list <owner> <repo> <pr>"
    _list_threads "$@" | jq -c 'select(.isResolved == false)'
    ;;
  list-all)
    [ $# -eq 3 ] || die "usage: list-all <owner> <repo> <pr>"
    _list_threads "$@"
    ;;
  resolve)
    [ $# -eq 1 ] || die "usage: resolve <threadId>"
    _resolve "$1"
    ;;
  unresolve)
    [ $# -eq 1 ] || die "usage: unresolve <threadId>"
    _unresolve "$1"
    ;;
  resolve-all)
    [ $# -ge 3 ] || die "usage: resolve-all <owner> <repo> <pr> [author]"
    owner=$1 repo=$2 pr=$3 author=${4:-}
    n=0
    while IFS= read -r id; do
      [ -n "$id" ] || continue
      _resolve "$id"
      n=$((n+1))
    done < <(
      _list_threads "$owner" "$repo" "$pr" \
        | jq -r --arg a "$author" '
            select(.isResolved == false)
            | select($a == "" or .author == $a)
            | .id'
    )
    echo "resolved $n thread(s)"
    ;;
  *)
    grep '^#' "$0" | sed 's/^# \{0,1\}//'
    exit 1
    ;;
esac
