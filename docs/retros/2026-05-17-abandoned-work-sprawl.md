# DEAR Retro: Abandoned-Work Sprawl (Worktrees, Branches, PRs)

**Date:** 2026-05-17
**Severity:** High (data-loss risk on 4 unmerged items; ~90% husk worktree population)
**Status:** Open — audit complete, fixes proposed, not yet wired
**Lineage:** third occurrence — see [`2026-05-01-resource-cleanup.md`](2026-05-01-resource-cleanup.md) (tooling added, hook never wired) → recurrence 2026-05-15 → reaper landed PR #120 (2026-05-16) → **this** audit 2026-05-17 still finds ~46 worktrees
**Companion data:** [`agm/docs/ABANDONED-WORK-AUDIT-2026-05-17.md`](../../agm/docs/ABANDONED-WORK-AUDIT-2026-05-17.md)

---

## Define

**Invariant:** every session that creates a worktree/branch closes it out
when it ends — merged→reap, unmerged-valuable→push or PR,
unmerged-worthless→delete. No session leaves orphaned state behind.

**Observed violation:** ~46 worktrees across 6 repos, ~34 husks, **only
3–4 active**. Four items hold unmerged work at real risk of loss
(`feat/discord-multibot-portal`, brain-v2 & engram-research primary
checkouts parked off-main with dirty trees, ai-conversation-logs research
files with no remote). This is the **third** time this exact failure has
been retro'd.

---

## Audit — why work is being abandoned

Root causes, in order of impact:

1. **The 2026-05-01 fix shipped a design, not a wired enforcement.** The
   reaper script existed for two weeks before the Stop hook was actually
   wired (PR #120, 2026-05-16). Classic "retro proposes a hook, PR ships
   only the doc/script" gap. Even now, #120 reaps **on Stop** — it does
   nothing about the dozens of husks created *before* it landed, husks
   from sessions that were killed (no Stop fired), or non-AGM worktrees.
   The enforcement point is too narrow and has no backfill/sweep.

2. **Parallel-agent fan-out with no dedup or close-out.** The dominant
   pattern: one task → 5–8 worktrees all with the *same commit subject*
   (task-accountability ×5 in brain-v2, "#16 testing audit" ×8 +
   "#24 relocate" ×4 in engram-research, #110/#114 dups ×5 in dear-agent).
   Many agents are launched for one logical task; one PR lands; the rest
   are never told they lost the race and never self-clean. New worktrees
   literally appeared *during this audit* (`objective-diffie-c0b2f6`).

3. **Squash-merge erases the "am I merged?" signal.** After squash,
   `git rev-list origin/main..branch` shows `ahead=143` for a branch with
   zero unique work. An agent (or a naive cleanup script) checking
   ahead/behind concludes "unmerged work — keep", so husks accumulate and
   nobody dares delete them. The only reliable signal is the PR head-ref
   map, which no automated cleanup consults.

4. **Primary checkouts used as scratch space.** brain-v2's `~/src`
   checkout sits on `feat/task-accountability` (38 behind origin, 40 dirty
   files); engram-research `~/src` main is `ahead=3 behind=17` with
   uncommitted deletions. Work-in-progress was left on the canonical
   checkout instead of a worktree, then a context reset / new task moved
   on. This is the highest data-loss risk and is invisible to any
   worktree-only reaper.

5. **PR backlog has no TTL.** dear-agent carries 12 open PRs, several
   untouched since 05-10/05-11 (only a bulk 05-16 touch). #118/#119 are
   *known-dead* dups of the merged #120 and still sit open. Nothing nudges
   a PR toward merge-or-close.

6. **ai-conversation-logs has no remote.** Abandonment there isn't
   recoverable — there's no origin and no off-box backup. 8 uncommitted
   research files are one disk failure from gone.

Not a significant cause: permission blocks. The evidence points to
*fan-out without close-out* and *enforcement wired at one narrow point*,
not to agents being denied the ability to clean up.

---

## Retro — systemic fixes

Ranked by leverage. **Each fix must ship its enforcement wired in the
same PR** — that is the lesson of the 05-01 retro, and it is now a
hard precondition for closing this one.

1. **Backfill sweep, not just on-Stop reap.** Add an idempotent
   `agm worktree sweep` (and a daily cron / SessionStart trigger) that
   reaps husks by the **authoritative rule**: branch has a *merged* PR
   (via `gh pr list --head`) OR is byte-identical to origin/main, AND
   worktree is clean, AND no live AGM/tmux owner. Allowlist semantics
   (only delete what positively proves reapable), never denylist. This
   covers the pre-existing backlog the Stop hook structurally cannot.

2. **PR-head-ref as the merge oracle.** Cleanup tooling must *never* use
   ahead/behind on squash repos. Encode "merged ⇔ `gh pr` head-ref state
   == MERGED" as a shared helper and use it everywhere. Add the
   merge-tree false-safe guard for verbatim file-split PRs (cf. #106).

3. **One task → one worktree.** The fan-out is the source. Either
   deduplicate at spawn (a task key → reuse existing worktree) or have
   losing parallel agents self-delete on completion when their branch
   didn't become the merged PR. Track this as an AGM feature, not a
   cleanup afterthought.

4. **Mandatory DEAR close-out gate.** The DEAR loop's Retro step must
   assert: no worktree/branch left for this task except the merged one;
   unmerged-valuable work has a pushed branch or open PR. Make it a Stop
   hook check that *reports* (not silently passes) when it leaves state
   behind, so the next session sees it.

5. **Worktree TTL + dashboard.** Tag each worktree with creation time and
   owning session; `agm worktree sweep` warns at 72h idle, the audit doc
   becomes a generated artifact (this file by hand today; automate it).

6. **PR TTL nudge.** Weekly job: any PR with no commits and no review
   activity in 7d gets a "merge or close" comment; auto-close known-dead
   classes (e.g. superseded reaper dups #118/#119). ai-tools: archive the
   repo so its 18 zombie PRs stop counting.

7. **Give ai-conversation-logs a remote** (or a scheduled off-box
   snapshot). An abandonment-prevention program is meaningless for the
   one repo where abandonment is unrecoverable.

---

## Action items (status)

| # | Action | Owner | Status |
|---|---|---|---|
| 1 | Save §3 unmerged work (discord-portal PR; brain-v2 + engram WIP; commit ACL research files) | human | **open — do before any cleanup** |
| 2 | Close dear-agent PR #118 & #119 (never merge — superseded by #120) | human | open |
| 3 | `agm worktree sweep` (backfill, PR-head oracle, allowlist) — **ship wired** | agm | open |
| 4 | One-task-one-worktree dedup / loser self-delete | agm | open |
| 5 | DEAR close-out Stop-hook assertion (reports leftover state) | agm | open |
| 6 | PR-TTL nudge + ai-tools repo archive | human/ci | open |
| 7 | ai-conversation-logs: add remote or scheduled backup | human | open |
| 8 | Allowlisted husk reap (~32 worktrees, 46→~14) per audit §4 | human | open — gated on #1 |

**Close-out precondition for this retro:** items 3–5 do not count as done
until their enforcement is merged and demonstrated reaping a real husk —
no repeat of the 05-01 "design merged, hook never wired" failure.
