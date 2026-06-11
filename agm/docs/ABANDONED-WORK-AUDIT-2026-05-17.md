# Abandoned-Work Audit — 2026-05-17

Cross-repo sweep of worktrees, branches, PRs, stashes and uncommitted
changes across the six tracked repos. Goal: surface work that was started
and never closed out, separate **stale husks** (safe to reap) from
**unmerged work worth saving**, and quantify the sprawl.

> **Nothing was deleted by this audit.** All disposition columns are
> recommendations. Cleanup commands are listed at the end; they are
> deliberately conservative because the existing `cleanup-worktrees.sh
> --fix` has no dirty-check and deletes remote branches.

Method note: dear-agent / engram-research land PRs via **squash-merge**,
which inflates `git rev-list origin/main..branch` (a branch cut from old
main shows `ahead=143 behind=144` while containing zero unique work). So
disposition is driven by the **authoritative PR head-ref mapping**
(`gh pr list`) and per-file diffs, *not* raw ahead/behind.

---

## 1. Headline numbers

| Metric | Count |
|---|---|
| Worktrees across all repos (excl. 6 primary checkouts) | **~46** (+3 phantom prunable in dear-agent) |
| Worktrees tied to an **open PR** | ~12 |
| Worktrees plausibly **active right now** | **3–4** |
| Worktrees that are **stale/empty/dup husks** | **~34** |
| Open PRs total | dear-agent 12 · brain-v2 11 (9 dependabot) · engram 3 · ai-tools 18 (all April, deprecated) · gdoc-sync 0 |
| Stashes | brain-v2 6 · engram-research 9 (mostly `auto-sync` automation noise) · dear-agent 1 |
| Primary checkouts NOT on a clean tracked branch | **3** (brain-v2, engram-research, ai-conversation-logs) |
| **Genuinely abandoned unmerged work worth saving** | **4 items** (see §3) |

**Ratio: ~46 worktrees exist; 3–4 are needed.** Roughly **90%** of the
worktree population is abandoned husk.

---

## 2. Abandoned-artifact table

Status legend: `EMPTY` = branch sits exactly on main, zero work ·
`MERGED-HUSK` = work landed via squash PR, branch/worktree leftover ·
`STALE-DUP` = parallel duplicate of a merged or active effort ·
`OPEN-PR(active|stale)` · `ABANDONED-UNMERGED` = unique work, no PR,
worth saving · `PRUNABLE` = worktree dir missing · `DIRTY-WIP` =
uncommitted changes on a primary checkout.

### dear-agent (`~/worktrees/dear-agent/**`)

| Worktree / branch | Last commit | Status | What it was |
|---|---|---|---|
| `quirky-babbage-d79db7` | 05-16 | **ACTIVE (this session)** | this audit |
| `stoic-lichterman-395d74` | 05-17 | OPEN-PR #121 active | move `select-option` to `send` group |
| `amazing-hypatia-b6401b` | 05-15 | OPEN-PR #115 | swe-lite real-patch / SWE-bench predictions |
| `pr115-rebase` (`rebase/pr115`) | 05-17 | STALE-DUP of #115 | duplicate rebase worktree for the same PR |
| `fix-pr108` (`feat/handoff-confidence-rebased`) | 05-15 | OPEN-PR #108 | gateway handoff confidence levels |
| `godfile-split` (`claude/godfile-split-new-go`) | 05-13 | OPEN-PR #106 ⚠ | new.go split — **merge-tree FALSE-SAFE**: verify `git log <merge-base>..main -- agm/cmd/agm/new.go` before merge |
| `trust-inversion` (`claude/trust-inversion-65`) | 05-10 | OPEN-PR #94 **stale** (7d) | trust-inversion verified_at seam |
| `epic-gauss-3172e2/.worktrees/{0151d989,cbaeb821,ec557f41}` | — | **PRUNABLE** (dirs missing) | nested broken AGM sub-worktrees; PR #93 |
| `jolly-hoover-ea8c03` | 05-16 | **CLOSE PR #119** — never merge | conservative-regression dup of #120 (reaper already landed) |
| `claude/wire-stop-hook-reaper` (no worktree) | 05-16 | **CLOSE PR #118** — never merge | bad-base dup of #120 |
| `admiring-moore-719861` | 05-15 | MERGED-HUSK | #114 landed via `fix/apfs-provider-wiring-bl011` |
| `beautiful-mestorf-8e586e` | 05-16 | MERGED-HUSK | #110 landed via `infallible-goldberg` |
| `friendly-cannon-c39762` | 05-16 | MERGED-HUSK | dup of merged #110 |
| `romantic-davinci-4d8a40` | 05-16 | MERGED-HUSK | dup of merged #110 |
| `infallible-goldberg-3576cb` | 05-15 | MERGED-HUSK | **was** head of merged PR #110 |
| `amazing-bartik-819bba` | 05-16 | EMPTY | at #120 HEAD, no unique work |
| `beautiful-pare-7d7b0f` | 05-16 | EMPTY | at #120 HEAD |
| `inspiring-yonath-b395f1` | 05-16 | EMPTY | at #120 HEAD |
| `reverent-torvalds-5405fc` | 05-16 | EMPTY | at #120 HEAD |
| `objective-diffie-c0b2f6` | 05-15 | STALE-DUP (appeared mid-audit) | #114 husk; concurrent-agent churn, dirty=1 |
| **`discord-portal` (`feat/discord-multibot-portal`)** | 05-16 | **ABANDONED-UNMERGED — SAVE** | multi-bot Discord portal, ADR-026, 1194+ LOC, never pushed, no PR |

Stale **open PRs with no worktree** (started, parked): #107
(`nice-grothendieck` delegation rules), #103 (`nice-chaum` 05-13 code
audit), #97 (`loving-elion` daily-audit workflows), #95
(`mystifying-stonebraker` weekly canaries), #93 (`epic-gauss`
constitutional designer — its worktree is the broken nested one).

### brain-v2 (`~/src/brain-v2` + `~/worktrees/brain-v2/**`)

| Worktree / branch | Last commit | Status | What it was |
|---|---|---|---|
| **`~/src/brain-v2` (`feat/task-accountability`)** | 05-07 | **DIRTY-WIP — primary checkout off main, 40 dirty files** | task-accountability module deleted+re-untracked; ADR-006, `docs/dear-reflections/`, `docs/research-notes/` uncommitted; never pushed, no PR |
| `happy-ardinghelli-4eb093` | 05-12 | OPEN-PR #51 **stale** | accountability matcher test + lint debt |
| `fervent-thompson-26fe62` | 05-07 | STALE-DUP | parallel copy of task-accountability |
| `infallible-mcnulty-5fdff1` | 05-07 | STALE-DUP | parallel copy of task-accountability |
| `stupefied-chatterjee-6ac873` | 05-07 | STALE-DUP | parallel copy of task-accountability |
| `unruffled-edison-5b3113` | 05-07 | STALE-DUP | parallel copy of task-accountability |
| `security-scan-deepsec` | 05-14 | MERGED-HUSK | DEAR Rules 5-8 landed via #52 (`condescending-diffie`) |

Stale PR: **#50 `angry-banzai`** (blog-feeds RSS poller, since 05-10, no
worktree). #53–#61 are dependabot (opened 05-17, current — not abandoned).

### engram-research (`~/src/engram-research` + `~/worktrees/engram-research/**`)

| Worktree / branch | Last commit | Status | What it was |
|---|---|---|---|
| **`~/src/engram-research` (main)** | 05-15 | **DIRTY-WIP** — local main `ahead=3 behind=17` of origin; uncommitted deletion of `substrate-hypothesis/*`, `+projects/agent-trust-protocol/` | research checkout diverged & not pushed |
| `gh-repo-audit-pr` (`research/github-repo-audit`) | 05-16 | OPEN-PR #24 active | GitHub repo audit |
| `remove-dead-orchestrator-workflow` | 05-12 | OPEN-PR #23 **stale** | remove dead orchestrator workflow |
| `brave-blackwell-201c13` | 05-07 | OPEN-PR #19 **stale** (9d) | DEAR↔Böckeler harness mapping |
| `architecture-audit-2026-05-13` (`research/dear-agent-architecture-audit-2026-05-13`) | 05-03 | MERGED-HUSK | testing audit landed via #16 |
| `compassionate-torvalds-5debfb` | 05-03 | MERGED-HUSK | dup of #16 testing audit |
| `epic-lalande-1251f4` | 05-03 | MERGED-HUSK | dup of #16 |
| `quirky-franklin-e535e8` | 05-03 | MERGED-HUSK | dup of #16 |
| `trusting-carson-7662de` | 05-03 | MERGED-HUSK | dup of #16 |
| `vibrant-hawking-93d68d` | 05-03 | MERGED-HUSK | dup of #16 |
| `vigorous-bouman-4b68c3` | 05-03 | MERGED-HUSK | dup of #16 |
| `vigorous-gates-105f3b` | 05-03 | MERGED-HUSK | dup of #16 |
| `determined-torvalds-3de0aa` | 05-15 | STALE-DUP of #24 | parallel "relocate repo audit" |
| `quirky-kapitsa-df77e3` | 05-15 | STALE-DUP of #24 | parallel "relocate repo audit" |
| `github-repo-audit-2026-05-15` (`research/github-repo-audit-2026-05-15`) | 05-15 | STALE-DUP of #24 | parallel "relocate repo audit" |
| `rebase-audit-docs` | 05-17 | STALE-DUP of #24 | parallel "relocate repo audit" |
| `hopeful-haslett-40aea0` | 05-15 | ABANDONED (pushed, no PR) — verify | DEAR retros / repo audits / workflow research; likely superseded by #5/#16/#18 |

### ai-conversation-logs (`~/ai-conversation-logs` — **no git remote**)

| Worktree / branch | Last commit | Status | What it was |
|---|---|---|---|
| **`~/ai-conversation-logs` (main)** | 05-17 | **DIRTY-WIP — 8 untracked research/audit `.md`, no remote backup** | agent-state-audit, beads-schema-analysis, model-routing-tier-spec, agent-federation synthesis, etc. |
| `angry-kalam-460134` | 05-15 | MERGED-HUSK | "rescue unversioned files" (already in main) |
| `laughing-visvesvaraya-82753b` | 05-15 | MERGED-HUSK | dup rescue task |
| `reverent-cartwright-332801` | 05-15 | MERGED-HUSK | dup rescue task |

### gdoc-sync

| Worktree / branch | Last commit | Status | What it was |
|---|---|---|---|
| `vibrant-lumiere-9ca15e` | 05-09 | MERGED-HUSK | tab-title cap; landed via PR #2 |

### ai-tools (deprecated — superseded by dear-agent/engram-research)

No worktrees, clean local. **18 open PRs all from 2026-04** (16
dependabot + #21 DEAR-PROTOCOL doc + #22 gc-removal). Repo's content was
migrated (engram PR #8 `migration-from-ai-tools`). Entire PR set is
abandoned-by-deprecation.

---

## 3. Unmerged work worth saving (do NOT reap)

1. **dear-agent `feat/discord-multibot-portal`** — real feature: ADR-026
   + `agm/internal/bus/discord_multibot.go` (506 LOC) +
   `discord_multibot_config.go` + 394 LOC of tests + agm-bus wiring
   (1194 insertions). One commit, **never pushed, no PR**. → Open a PR
   from this branch, or push it before any worktree cleanup.

2. **brain-v2 `feat/task-accountability` (the primary `~/src` checkout)**
   — task-accountability module, `docs/adr/006-task-accountability.md`,
   `docs/dear-reflections/`, `docs/research-notes/` all uncommitted; the
   canonical checkout is parked on a stale branch 38 behind origin/main.
   → Decide: land it as a PR or stash+restore main. Highest data-loss
   risk because it's the working copy everyone else branches from.

3. **engram-research local `main`** — `ahead=3 behind=17` of origin with
   uncommitted deletion of `research/2026-05-02-substrate-hypothesis/*`
   and an untracked `projects/agent-trust-protocol/`. → Reconcile with
   origin and push the 3 commits before they're lost to a rollback.

4. **ai-conversation-logs `main`** — 8 untracked research `.md` files in
   a repo **with no remote at all**. Single disk failure = total loss.
   → Commit them; this repo urgently needs an origin or an off-box backup.

Lower-confidence / verify-first: engram `hopeful-haslett-40aea0`
(pushed, no PR — diff it against `#5/#16/#18` before deleting);
dear-agent stash@{0} (`fix-broken-windows` lint WIP); brain-v2
stash@{2} hybrid-search WIP, stash@{3} linkedin dossier, stash@{5}
ABOUT-ME trim; engram stash@{2} agm-apfs retro. The numerous
`auto-sync-*` and `RETROSPECTIVE.md` case-collision stashes are
automation noise, safe to drop.

---

## 4. Recommendations

**Reap now (zero-risk husks)** — EMPTY + MERGED-HUSK only, after
confirming each has no PR and no dirty files:

- dear-agent: `amazing-bartik`, `beautiful-pare`, `inspiring-yonath`,
  `reverent-torvalds` (EMPTY); `admiring-moore`, `beautiful-mestorf`,
  `friendly-cannon`, `romantic-davinci`, `infallible-goldberg`
  (MERGED-HUSK); `git worktree prune` for the 3 phantom epic-gauss dirs.
- engram-research: the 8 `#16` MERGED-HUSKs + the 4 `#24` STALE-DUPs.
- brain-v2: `fervent-thompson`, `infallible-mcnulty`,
  `stupefied-chatterjee`, `unruffled-edison`, `security-scan-deepsec`.
- ai-conversation-logs: all 3 rescue husks. gdoc-sync: `vibrant-lumiere`.

That single pass removes **~32 worktrees** and takes the population from
~46 → ~14.

**Close, do not merge:** dear-agent PR **#118** and **#119** (both
superseded by the landed #120 reaper). Triage stale PRs #94, #95, #97,
#103, #107 (decide: revive or close). engram PRs #19, #23 (stale).
brain-v2 #50, #51. ai-tools: archive the repo or bulk-close all 18 PRs.

**Save first (see §3):** push/PR `feat/discord-multibot-portal`;
resolve the brain-v2 and engram-research primary-checkout WIP; commit
the ai-conversation-logs research files and give that repo a remote.

**Do not use `cleanup-worktrees.sh --fix` blindly** — it has no
dirty-check and deletes remote branches. Reap by **allowlist** (the
husk list above), never denylist. Verify each with
`gh pr list --head <branch>` + `git -C <wt> status --porcelain` +
`git cherry origin/main <branch>` immediately before deletion; the
"safe" buckets here are derived, not authoritative — re-derive at
delete time.

Suggested per-worktree guard (run for each husk before `git worktree
remove`):

```sh
wt=...; br=$(git -C "$wt" rev-parse --abbrev-ref HEAD)
[ -z "$(git -C "$wt" status --porcelain)" ] \
  && [ -z "$(gh pr list -R vbonnet/dear-agent --head "$br" --json number -q '.[].number')" ] \
  && echo "REAPABLE: $wt ($br)" || echo "HOLD: $wt ($br)"
```

See the companion DEAR retro:
[`docs/retros/2026-05-17-abandoned-work-sprawl.md`](../../docs/retros/2026-05-17-abandoned-work-sprawl.md).
