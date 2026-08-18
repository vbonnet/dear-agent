# ADR-033: Commit-Anchored Progress Ledger for Long-Running Workers

Status: Accepted (2026-06-21) · Bead ce-ek4f · advisory 2026-06-21 ID-002 · relates [018](ADR-018-graceful-exit-framework-default.md)

A long-running Worker that compacts or restarts loses its in-flight progress:
the context window is lossy, the bead note is written once at the end, and a
deliberate handoff to a new session does not resume the same one. Between
those, a Worker mid-task has no durable, granular record of "what's done, what's
next." It re-derives state from a summary — and re-derivation drifts.

**Decision.** Each significant milestone gets its own git commit on the Worker's
branch, with a structured trailer the Worker can parse back. The **commit
history *is* the progress ledger**. On compaction or restart, the Worker runs
`git log` (not its context window) to reconstruct what it has done, and
`git show` to recover *how*. The ledger trailer rides existing trailers
(`Co-Authored-By`, `Claude-Session`):

```
wip(ce-xxxx): <milestone summary>

Ledger-Milestone: <n>/<total or ?> — <what this commit completed>
Ledger-Next: <the single next action a resumed worker should take>
Ledger-Bead: ce-xxxx
```

Why a commit and not a file: a commit binds the progress claim to an actual tree
SHA, so the ledger **cannot lie about what code exists** — it is immune to a
stale prose-manifest failure mode.
A prose ledger drifts from the tree; a commit *is* the tree.

The noise cost is bounded by the existing workflow: per-milestone WIP commits
live on the feature branch and collapse under `git merge --squash`, so `main`
history stays clean. The ledger is branch-local and ephemeral by construction.

**Rejected.** *Context window only* — lossy, the problem statement. *Bead note
only* — coarse (one terminal write) and a network/DB round-trip per update, too
expensive to be per-milestone. *External ledger file* (`.progress.md`) — drifts
from the tree (the §H6 failure mode) and itself needs committing to survive.
*Handoff manifest* — heavyweight and semantically wrong: it models a cut to a
*new* session, whereas this models continuity *within* one. The ledger
complements bead notes (coarse, cross-session, queryable) by being granular and
in-band; it complements handoff (deliberate cut) by being for resumption.

Resume contract: a Worker's **first action** after compaction/restart is to read
its own ledger off the branch, then act on the most recent `Ledger-Next`. Read it
from the **merge-base** with the base branch, not `origin/main` directly:

```
base=$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || echo origin/main)
git log --format=%B "$(git merge-base HEAD "$base")"..HEAD
```

Merge-base, not a bare `origin/main..HEAD`, because the latter assumes the remote
is named `origin`, the default branch is `main`, and the remote ref is freshly
fetched — none guaranteed offline. Resolving `origin/HEAD` recovers the real
default branch (`master` or otherwise), and the merge-base anchors to the actual
fork point, so a stale remote ref can never resurface already-merged commits. The
Worker tolerates empty output (a fresh branch with no milestones yet) and a
non-zero exit (no resolvable base) by falling back to the bead note. Verified by
the resumed Worker reaching the bead's acceptance criteria without re-doing
committed milestones.
