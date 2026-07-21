#!/usr/bin/env bash
# cleanup-worktrees.sh — find and remove stale git worktrees for a repo.
#
# A worktree is "stale" if either:
#   - its branch has 0 commits ahead of origin/main (already merged or empty)
#   - its branch has had no commits in 14+ days (configurable via --max-age)
#
# Default behavior is dry-run: it prints what it would do and exits.
# Pass --fix to actually remove worktrees + local branches + remote branches.
#
# Usage:
#   scripts/cleanup-worktrees.sh <repo-path> [--fix] [--max-age DAYS] [--preserve NAME ...]
#
# Examples:
#   # Audit dear-agent worktrees (read-only)
#   scripts/cleanup-worktrees.sh ~/src/dear-agent
#
#   # Audit and remove stale ones, but keep three named worktrees
#   scripts/cleanup-worktrees.sh ~/src/dear-agent --fix \
#       --preserve quirky-pare-496fcb \
#       --preserve silly-burnell-4daa27 \
#       --preserve pr-31-huh
#
# Exit codes:
#   0  success (or no stale worktrees in dry-run)
#   1  usage / argument error
#   2  repo path is not a valid git directory
#   3  one or more removals failed in --fix mode

set -euo pipefail

export GIT_TERMINAL_PROMPT=0

usage() {
    cat >&2 <<EOF
Usage: $0 <repo-path> [--fix] [--max-age DAYS] [--preserve NAME ...]

Lists git worktrees for the given repo, identifies stale ones (branch
merged to origin/main or no commits in DAYS days), and prints what would
be done. Pass --fix to actually remove them.

Options:
  --fix              remove stale worktrees, local branches, and remote branches
  --max-age DAYS     idle threshold in days (default: 14)
  --preserve NAME    worktree directory basename to keep (repeatable)
  -h, --help         this message
EOF
}

REPO=""
DO_FIX=0
MAX_AGE=14
PRESERVE=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage
            exit 0
            ;;
        --fix)
            DO_FIX=1
            shift
            ;;
        --max-age)
            [[ $# -lt 2 ]] && { echo "error: --max-age needs a value" >&2; exit 1; }
            MAX_AGE="$2"
            shift 2
            ;;
        --preserve)
            [[ $# -lt 2 ]] && { echo "error: --preserve needs a value" >&2; exit 1; }
            PRESERVE+=("$2")
            shift 2
            ;;
        --)
            shift
            break
            ;;
        -*)
            echo "error: unknown flag: $1" >&2
            usage
            exit 1
            ;;
        *)
            if [[ -z "$REPO" ]]; then
                REPO="$1"
                shift
            else
                echo "error: unexpected argument: $1" >&2
                exit 1
            fi
            ;;
    esac
done

if [[ -z "$REPO" ]]; then
    usage
    exit 1
fi

if ! git -C "$REPO" rev-parse --git-dir >/dev/null 2>&1; then
    echo "error: not a git directory: $REPO" >&2
    exit 2
fi

log() {
    printf '[cleanup-worktrees] %s\n' "$*"
}

is_preserved() {
    local name="$1"
    local p
    for p in "${PRESERVE[@]+"${PRESERVE[@]}"}"; do
        [[ "$name" == "$p" ]] && return 0
    done
    return 1
}

# Resolve target ref: prefer origin/HEAD, fall back to origin/main.
TARGET_REF=""
if git -C "$REPO" rev-parse --verify --quiet origin/HEAD >/dev/null 2>&1; then
    TARGET_REF="$(git -C "$REPO" symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || true)"
fi
if [[ -z "$TARGET_REF" ]]; then
    TARGET_REF="origin/main"
fi
if ! git -C "$REPO" rev-parse --verify --quiet "$TARGET_REF" >/dev/null 2>&1; then
    echo "error: target ref not found: $TARGET_REF" >&2
    exit 2
fi

log "repo: $REPO"
log "target ref: $TARGET_REF"
log "max-age: $MAX_AGE days"
log "mode: $([[ $DO_FIX -eq 1 ]] && echo FIX || echo DRY-RUN)"

# Refresh remote so the merged-to-main check reflects reality.
git -C "$REPO" fetch --quiet origin 2>/dev/null || log "warning: fetch failed; using cached refs"

NOW_EPOCH="$(date +%s)"
MAX_AGE_SEC=$(( MAX_AGE * 86400 ))

STALE_COUNT=0
PRESERVED_COUNT=0
KEPT_COUNT=0
FAILED_COUNT=0

# Parse `git worktree list --porcelain`. Records are blank-line-separated
# and start with `worktree <path>`.
worktree_path=""
worktree_branch=""
worktree_head=""
worktree_bare=0
worktree_detached=0

flush_record() {
    [[ -z "$worktree_path" ]] && return 0

    # Reset state for the next record on return.
    local path="$worktree_path"
    local branch="$worktree_branch"
    local head="$worktree_head"
    local bare="$worktree_bare"
    local detached="$worktree_detached"
    worktree_path=""
    worktree_branch=""
    worktree_head=""
    worktree_bare=0
    worktree_detached=0

    # Skip the main checkout (the repo path itself), bare, and detached.
    if [[ "$path" == "$REPO" ]] || [[ "$bare" -eq 1 ]] || [[ "$detached" -eq 1 ]]; then
        return 0
    fi

    local name
    name="$(basename "$path")"

    if is_preserved "$name"; then
        log "preserve: $name (--preserve)"
        PRESERVED_COUNT=$(( PRESERVED_COUNT + 1 ))
        return 0
    fi

    # Number of commits on the branch but not on the target ref.
    local ahead=0
    if [[ -n "$head" ]]; then
        ahead="$(git -C "$REPO" rev-list --count "$TARGET_REF..$head" 2>/dev/null || echo 0)"
    fi

    # Last commit time on the branch (epoch seconds).
    local last_commit_epoch=0
    if [[ -n "$head" ]]; then
        last_commit_epoch="$(git -C "$REPO" log -1 --format=%ct "$head" 2>/dev/null || echo 0)"
    fi
    local age_sec=$(( NOW_EPOCH - last_commit_epoch ))
    local age_days=$(( age_sec / 86400 ))

    local reason=""
    if [[ "$ahead" == "0" ]]; then
        reason="merged-or-empty (0 commits ahead of $TARGET_REF)"
    elif [[ "$age_sec" -ge "$MAX_AGE_SEC" ]]; then
        reason="idle ($age_days days since last commit, ahead=$ahead)"
    fi

    if [[ -z "$reason" ]]; then
        KEPT_COUNT=$(( KEPT_COUNT + 1 ))
        return 0
    fi

    STALE_COUNT=$(( STALE_COUNT + 1 ))
    log "stale: $name [$branch] — $reason"

    if [[ $DO_FIX -ne 1 ]]; then
        log "  would: git -C $REPO worktree remove --force $path"
        log "  would: git -C $REPO branch -D $branch"
        log "  would: git -C $REPO push origin --delete $branch"
        return 0
    fi

    local local_failed=0

    if ! git -C "$REPO" worktree remove --force "$path" 2>&1 | sed 's/^/    /'; then
        log "  FAILED: worktree remove $path"
        log "  skipped branch cleanup for $branch because the worktree was preserved"
        FAILED_COUNT=$(( FAILED_COUNT + 1 ))
        return 0
    else
        log "  removed worktree: $path"
    fi

    if [[ -n "$branch" ]] && git -C "$REPO" rev-parse --verify --quiet "$branch" >/dev/null 2>&1; then
        if ! git -C "$REPO" branch -D "$branch" 2>&1 | sed 's/^/    /'; then
            log "  FAILED: branch -D $branch"
            local_failed=1
        else
            log "  deleted local branch: $branch"
        fi
    fi

    if [[ $local_failed -eq 1 ]]; then
        log "  skipped remote branch cleanup for $branch because local cleanup failed"
        FAILED_COUNT=$(( FAILED_COUNT + 1 ))
        return 0
    fi

    if [[ -n "$branch" ]]; then
        # Best-effort remote delete; tolerate already-deleted upstream.
        if git -C "$REPO" push origin --delete "$branch" 2>&1 | sed 's/^/    /'; then
            log "  deleted remote branch: $branch"
        else
            log "  note: remote branch $branch already gone or push failed"
        fi
    fi

}

while IFS= read -r line; do
    if [[ -z "$line" ]]; then
        flush_record
        continue
    fi
    case "$line" in
        worktree\ *)
            worktree_path="${line#worktree }"
            ;;
        HEAD\ *)
            worktree_head="${line#HEAD }"
            ;;
        branch\ *)
            # branch is given as refs/heads/<name>
            worktree_branch="${line#branch refs/heads/}"
            ;;
        bare)
            worktree_bare=1
            ;;
        detached)
            worktree_detached=1
            ;;
    esac
done < <(git -C "$REPO" worktree list --porcelain)
flush_record

log "summary: stale=$STALE_COUNT kept=$KEPT_COUNT preserved=$PRESERVED_COUNT failed=$FAILED_COUNT"

# Prune deleted worktree metadata (cheap, safe even in dry-run).
git -C "$REPO" worktree prune

if [[ $FAILED_COUNT -gt 0 ]]; then
    exit 3
fi
