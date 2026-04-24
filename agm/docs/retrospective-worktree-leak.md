# Retrospective: Worktree Leak Incident

**Date**: 2026-04-10
**Severity**: Medium (disk exhaustion risk)
**Impact**: ~57G disk consumed by 404 orphaned worktrees across ai-tools repo

## Summary

Over time, AGM worker sessions created git worktrees for task isolation but
never cleaned them up on exit. This accumulated 404 orphaned worktrees
consuming ~57G of disk space (29G in ai-tools worktrees alone).

## Timeline

- **Discovery**: 2026-04-10 — disk pressure investigation revealed ~172+
  worktrees (actual count: 404 including nested .worktrees)
- **Remediation**: Same day — DEAR cycle removed 333 worktrees, pruned stale
  references, ran git gc. Reduced to 68 worktrees (12 active sessions +
  56 with unmerged commits preserved). Disk freed: ~20G.

## Categories of Orphaned Worktrees

| Category | Count | Action |
|----------|-------|--------|
| Prunable (sandbox already deleted) | 91 | Removed via `git worktree prune` |
| Fully merged into main | 244 | Removed via `git worktree remove --force` |
| Unmerged commits (preserved) | 57 | Left in place — need manual review |
| Active sessions (protected) | 12 | Not touched |

## Root Cause Analysis

### Primary: Worker exit does not clean up worktrees

The `/agm:agm-exit` skill archives the session and cleans up AGM metadata, but
does **not** remove the git worktree created during the session. Each worker
session creates 1-N worktrees (main + nested sub-agent worktrees) that persist
indefinitely after the session ends.

### Contributing factors

1. **No worktree lifecycle tracking**: AGM does not record which worktrees
   belong to which session, making post-hoc cleanup difficult.
2. **Nested worktrees**: Some sessions (e.g., overseer, ci-auditor) create
   sub-worktrees inside their main worktree, amplifying the leak.
3. **No periodic GC**: There is no cron job, daemon task, or session hook that
   periodically prunes stale worktrees.
4. **Sandbox overlay masking**: Sandbox-based worktrees (under
   `~/.agm/sandboxes/`) appear in `git worktree list` but cannot be removed
   with `git worktree remove` since they're overlay mounts — only `prune` works.

## Why Wasn't This Auto-Detected?

1. **No disk monitoring**: AGM has no health check for worktree count or disk
   usage in the repo's worktree directory.
2. **`agm admin doctor`** does not audit worktree state.
3. **Gradual accumulation**: Each session adds ~150MB (one worktree), so the
   leak is invisible until dozens of sessions have run.

## Recommended Fixes

### Short-term (P0)

- **Add worktree cleanup to `/agm:agm-exit` skill**: On session exit, identify
  and remove worktrees created by that session. For worktrees with unmerged
  commits, warn the user instead of silently removing.

### Medium-term (P1)

- **Add worktree tracking to session manifest**: Record worktree paths in the
  session manifest so cleanup is deterministic, not heuristic.
- **Add `agm admin gc-worktrees` command**: Standalone command that performs
  the DEAR cycle — list, check unmerged, cross-reference active sessions,
  remove safe ones.
- **Add worktree audit to `agm admin doctor`**: Flag sessions with orphaned
  worktrees.

### Long-term (P2)

- **Periodic GC via daemon**: The AGM daemon could periodically prune worktrees
  for archived sessions (e.g., daily, when worktree count exceeds threshold).
- **Disk budget enforcement**: Set a configurable limit on total worktree disk
  usage and alert or auto-clean when exceeded.

## Lessons Learned

1. Any resource created during a session lifecycle (worktrees, temp files,
   branches) needs a corresponding cleanup path on session exit.
2. Overlay filesystems add complexity — `git worktree remove` doesn't work on
   overlay paths, requiring `git worktree prune` instead.
3. Nested worktrees (worktrees-of-worktrees) multiply the leak rate and
   complicate cleanup — consider limiting nesting depth.
