#!/usr/bin/env bash
# git-sync-main.sh — atomic stash / fetch / rebase / pop onto origin's
# default branch.
#
# Replaces the unsafe "git stash; git pull --rebase; git stash pop"
# idiom that agents reach for when their branch is behind origin/main
# with local changes. The agent harness denies bare `git stash`
# invocations because they are almost always a workaround for trying to
# write to main with a dirty tree; this script is the sanctioned form
# for the legitimate case (sync a feature branch onto origin/main).
#
# Usage: scripts/git-sync-main.sh <repo-path>
#
# Behavior, in order:
#   1. Capture the starting branch and HEAD.
#   2. Resolve the target ref (origin/HEAD if available, else origin/main).
#   3. Fetch origin.
#   4. Stash tracked local changes, if any, with a uniquely-named stash.
#   5. Rebase the current branch onto the target ref.
#   6. Pop the stash, if one was created.
#   7. On rebase failure: abort the rebase and pop the stash so the
#      working tree returns to its starting state, then exit non-zero.
#
# Exit codes:
#   0  success (or no-op when already in sync with no local changes)
#   1  usage / argument error
#   2  rebase failed and was aborted; original state restored
#   3  stash pop produced conflicts; resolve manually (stash kept)
#   4  unexpected error (fetch failed, stash bookkeeping failed, etc.)

set -euo pipefail

usage() {
    cat >&2 <<EOF
Usage: $0 <repo-path>

Stashes tracked local changes (if any), fetches origin, rebases the
current branch onto origin/HEAD (or origin/main), then pops the stash.
On any failure the original state is restored.
EOF
}

if [ $# -ne 1 ]; then
    usage
    exit 1
fi

REPO_PATH=$1

if [ ! -d "$REPO_PATH" ]; then
    echo "git-sync-main: $REPO_PATH is not a directory" >&2
    exit 1
fi

if ! git -C "$REPO_PATH" rev-parse --git-dir >/dev/null 2>&1; then
    echo "git-sync-main: $REPO_PATH is not a git repository" >&2
    exit 1
fi

REPO_ABS=$(cd "$REPO_PATH" && pwd)

step() { echo "git-sync-main: $*"; }
err()  { echo "git-sync-main: ERROR: $*" >&2; }

# 1. Capture starting state.
START_BRANCH=$(git -C "$REPO_ABS" symbolic-ref --short --quiet HEAD || true)
if [ -z "$START_BRANCH" ]; then
    err "detached HEAD; refusing to rebase"
    exit 1
fi
START_SHA=$(git -C "$REPO_ABS" rev-parse HEAD)
step "branch=$START_BRANCH head=${START_SHA:0:12}"

# 2. Resolve target ref.
REMOTE_REF=
if git -C "$REPO_ABS" symbolic-ref --quiet refs/remotes/origin/HEAD >/dev/null 2>&1; then
    REMOTE_REF=$(git -C "$REPO_ABS" symbolic-ref --short refs/remotes/origin/HEAD)
elif git -C "$REPO_ABS" rev-parse --verify --quiet refs/remotes/origin/main >/dev/null; then
    REMOTE_REF=origin/main
else
    err "could not resolve origin/HEAD or origin/main; is 'origin' configured?"
    exit 1
fi
step "target=$REMOTE_REF"

# 3. Fetch (read-only — fail before touching the working tree).
step "fetching origin"
if ! git -C "$REPO_ABS" fetch origin; then
    err "fetch failed"
    exit 4
fi

# 4. Stash tracked changes, if any.
STASH_MSG="git-sync-main:$$:$(date -u +%Y%m%dT%H%M%SZ)"
DID_STASH=0

if ! git -C "$REPO_ABS" diff-index --quiet HEAD --; then
    step "stashing tracked local changes ($STASH_MSG)"
    if ! git -C "$REPO_ABS" stash push -m "$STASH_MSG" >/dev/null; then
        err "stash failed; aborting"
        exit 4
    fi
    DID_STASH=1
else
    step "working tree clean; no stash needed"
fi

# Locate our stash by message — guards against concurrent stash activity
# in the same repo (other shells, hooks, etc.).
find_our_stash() {
    git -C "$REPO_ABS" stash list | \
        awk -v m="$STASH_MSG" 'index($0, m) { sub(/:.*/, "", $0); print; exit }'
}

# 5. Rebase.
step "rebasing $START_BRANCH onto $REMOTE_REF"
REBASE_RC=0
git -C "$REPO_ABS" rebase "$REMOTE_REF" || REBASE_RC=$?

if [ "$REBASE_RC" -ne 0 ]; then
    err "rebase failed (rc=$REBASE_RC); aborting"
    git -C "$REPO_ABS" rebase --abort 2>/dev/null || true
    if [ "$DID_STASH" -eq 1 ]; then
        ref=$(find_our_stash)
        if [ -n "$ref" ]; then
            step "restoring stash $ref"
            if ! git -C "$REPO_ABS" stash pop "$ref"; then
                err "stash pop produced conflicts during rollback; stash kept"
                exit 3
            fi
        else
            err "stash created but not found in stash list; not restoring"
            exit 4
        fi
    fi
    exit 2
fi

NEW_SHA=$(git -C "$REPO_ABS" rev-parse HEAD)
if [ "$NEW_SHA" = "$START_SHA" ]; then
    step "already up to date"
else
    step "rebased: ${START_SHA:0:12} -> ${NEW_SHA:0:12}"
fi

# 6. Pop the stash, if we created one.
if [ "$DID_STASH" -eq 1 ]; then
    ref=$(find_our_stash)
    if [ -z "$ref" ]; then
        err "stash created but not found in stash list; nothing to pop"
        exit 4
    fi
    step "popping stash $ref"
    if ! git -C "$REPO_ABS" stash pop "$ref"; then
        err "stash pop produced conflicts; resolve manually (stash retained)"
        exit 3
    fi
fi

step "done"
