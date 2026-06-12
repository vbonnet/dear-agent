# DEAR Retro — ~/src Protected-Path Violations & Bead-Burndown Ineffectiveness

- **Date:** 2026-06-11
- **Branch:** `retro/src-violations`
- **Author:** burndown/governance agent
- **Scope:** two related incidents that share one root cause — scheduled tasks
  whose code-task workspace points at the read-only golden checkouts under
  `~/src/**` instead of a worktree.

> **TL;DR.** The `bead-burndown-loop` and `youtube-channel-poll` scheduled
> tasks both declare their working directory as `~/src/<repo>`. That single
> design choice (a) caused agents to run `git stash`/`pull`/`cd && python` writes
> directly in the golden checkouts (Incident 1), and (b) is part of why
> automated burndown has closed ~0 beads while the user keeps prompting manually
> (Incident 2). The fix is a narrow `src-recovery` wrapper to clean up the
> damage safely, plus pointing the scheduled tasks at worktrees and adding a
> VROOM Overseer to keep burndown concurrency alive.

---

## Incident 1 — `~/src/**` Protected-Path Violations

### Define

**What happened.** Both golden checkouts were found dirty and off-policy:

`~/src/dear-agent`:
- `git stash pop` conflict left in the working tree — `both modified`
  (`UU`) on `.claude/CLAUDE.md` and `AGENTS.why.md`. The conflict markers are
  labelled `Updated upstream` / `Stashed changes`, the signature of a
  `git stash pop`, over the new **Anti-Stall** CLAUDE.md section.
- Untracked work products written straight into the tree:
  `docs/retros/2026-05-09-chezmoi-drift.md`, `youtube-channel-poll-report.md`.
- Was 141 commits behind `origin/main` (since recovered to even by an
  automated `pull --rebase` 17 min before this session — see reflog).

`~/src/brain-v2`:
- On feature branch `feat/task-accountability` (a 5-week-old commit), **not**
  `main`, with **no upstream tracking**.
- `config/youtube-channels.yaml` modified in place.
- A large pile of untracked artifacts in the tree (`.agm/sessions/...`,
  built binaries `brain` and `cmd/brain/brain`, `docs/dear-reflections/`,
  `go.work.sum`, `.claude/`, etc.).

**Expected behaviour.** `~/src/**` is the read-only golden reference tree.
Every write — including every git mutation — must happen in `~/worktrees/**`.
The checkouts should always be on `main`, clean, and fast-forwardable.

**Rule violated.** The STRICT top-level rule in `~/.claude/CLAUDE.md`:
"`~/src/` contains the golden reference checkouts. **NO writes, ever.**" and the
project rule "All repo work happens in `~/worktrees/`, never directly in
`~/src/`." This is **not the first time** (the rule predates this incident and
exists precisely because it has happened before).

### Execute (Investigation)

**`~/src/dear-agent` reflog** (`git -C ~/src/dear-agent reflog --date=relative`):

```
c6b74a8771 HEAD@{17 minutes ago}: pull --rebase --quiet: Fast-forward
1ad4bdcf9f HEAD@{18 minutes ago}: reset: moving to HEAD
1ad4bdcf9f HEAD@{9 days ago}:    pull --rebase --quiet (finish): returning to refs/heads/main
95ad5a8f41 HEAD@{2 weeks ago}:   merge chezmoi-deploy: Fast-forward
6065ce348d HEAD@{2 weeks ago}:   reset: moving to HEAD     (×~20 in a row)
```

Two tells:
1. **`pull --rebase --quiet`** — the `--quiet` flag is a script signature, not a
   human at a prompt. Something automated pulls *inside the golden checkout*.
2. **Runs of `reset: moving to HEAD`** — repeated `git reset HEAD` (unstaging)
   loops, the fingerprint of an agent repeatedly staging/unstaging in-tree.

The working-tree conflict itself is a `git stash pop` artifact: an agent had
local edits to `.claude/CLAUDE.md` (the Anti-Stall section, which is also the
content shown stashed at session start), stashed them, pulled, and popped —
**all in `~/src/dear-agent`**.

**`~/src/brain-v2` reflog** shows `checkout: moving from main to
feat/task-accountability` 5 weeks ago and a 5-week-old in-tree commit
`feat(accountability): ...` — i.e. real feature work was committed directly to
the golden checkout and never cleaned up.

**Who created the changes — the smoking guns are the scheduled-task SKILLs:**

- `~/Documents/Claude/Scheduled/bead-burndown-loop/SKILL.md` declares:
  > "The workspace for code tasks is `/Users/vbonnet/src/dear-agent`"
  > "Start a code task in `/Users/vbonnet/src/dear-agent` to run `bd list`..."

  So every 3 hours the loop spawns code tasks **rooted in the golden checkout**.
  The prompt *tells* them "never write to `~/src/dear-agent` directly," but the
  cwd *is* `~/src/dear-agent`, so `bd`, `git status`, and any reflexive
  `git stash`/`pull` land there. The instruction fights the environment, and the
  environment wins.

- `~/Documents/Claude/Scheduled/youtube-channel-poll/SKILL.md` is worse — it
  declares the literal command:
  > `cd ~/src/brain-v2 && python3 scripts/poll-youtube-channels.py --verbose`

  and reads/writes `~/src/brain-v2/config/youtube-channels.yaml` and emits a
  report. That is the exact provenance of brain-v2's modified yaml and the
  `youtube-channel-poll-report.md` untracked in `~/src/dear-agent`'s root.
  (It also runs Python on code we control — a separate CLAUDE.md principle-4
  violation worth its own bead.)

### Audit

**Controls that should have prevented this, and why they failed:**

| Control | Why it failed here |
|---|---|
| `~/.claude/CLAUDE.md` STRICT "no writes to ~/src" rule | Instruction-only. A scheduled task whose **cwd is the golden checkout** routes around it by default — the agent doesn't "decide" to write to ~/src, it's already standing in it. |
| Bash write-guard / `git -C ~/src/*` deny-rules | Per `memory/src-readonly-guardrail-porous.md`: deny rules only match the `git -C ~/src/*` form. A scheduled task that does `cd ~/src/brain-v2 && git/python ...` with a blanket `Bash` allow is a **full bypass**. No FS-layer or git-hook backstop exists. |
| Worktree-only policy | Assumes the agent *starts* outside ~/src. The SKILL files hard-code ~/src as the start directory, so the policy never engages. |

**Is this a recurring pattern?** Yes — explicitly. The rule exists *because* it
recurred; `memory/src-readonly-guardrail-porous.md` already documents the
`cd && git` bypass; and the brain-v2 state is 5 weeks old, so it has been
sitting in violation across many sessions undetected.

### Retro

**Root cause.** Two scheduled-task SKILL files declare `~/src/<repo>` as the
code-task workspace. The read-only-`~/src` guard is *instruction + porous deny
rule* only, with no environment-level or FS-level enforcement and **no
sanctioned recovery path** — so once a checkout drifts, even cleaning it up
requires a forbidden raw git write into `~/src`, which agents either can't do or
route around.

**Proposed fixes.**

1. **(this PR) `src-recovery` wrapper** — the one vetted, narrowly-scoped writer
   to `~/src/**`. Validates the path is strictly under `~/src/`, then runs
   exactly `git stash --include-untracked` (if dirty) → `git checkout
   <default-branch>` → `git pull --ff-only`, refusing every other git verb by
   construction and logging each step to
   `~/.local/state/dear-agent/src-recovery.log`. `cmd/src-recovery` +
   `internal/safesrc`, mirroring `safe-push`. Allow-list `Bash(src-recovery *)`
   in chezmoi. (See "The Fix" below; dry-run already correctly refuses the live
   conflicted dear-agent tree.)

2. **Repoint the scheduled tasks at worktrees.** `bead-burndown-loop` and
   `youtube-channel-poll` SKILLs must set the code-task workspace to a
   `~/worktrees/<repo>-<purpose>` checkout, never `~/src/<repo>`. The bead-list
   read can run against a dedicated `~/worktrees/dear-agent-burndown-readonly`
   checkout. (Beads: `BD-SRC-2`, `BD-SRC-3`.)

3. **Close the `cd && git` bypass** at the guard layer, not just in prose:
   extend the Bash write-guard to also block `cd ~/src/... && <write>` and bare
   `git`/`python`/`>` writes when the resolved cwd is under `~/src/**`. (Bead:
   `BD-SRC-4`; tracks `memory/src-readonly-guardrail-porous.md`.)

4. **Recover the two checkouts** with `src-recovery` once #1 lands —
   dear-agent's conflict must be hand-resolved first (the tool correctly
   refuses a half-merged tree), then `src-recovery ~/src/dear-agent` and
   `src-recovery ~/src/brain-v2`. Preserve brain-v2's `feat/task-accountability`
   work to a real worktree branch before recovering if it is not yet upstream.
   (Bead: `BD-SRC-5`.)

**Beads filed:** `BD-SRC-1` (src-recovery, this PR), `BD-SRC-2`/`BD-SRC-3`
(repoint SKILLs), `BD-SRC-4` (guard the `cd && git` bypass), `BD-SRC-5`
(recover the two trees), `BD-SRC-6` (youtube-poll Python→Go).

---

## Incident 2 — Bead-Burndown Ineffectiveness

### Define

**What happened.** The `bead-burndown-loop` scheduled task runs every 3 hours
(`cron 0 */3 * * *`), yet the user still has to keep prompting for burndown
work by hand. Automated burndown is not, in practice, burning down the backlog.

**Expected behaviour.** The loop should keep up to 2–3 burndown code tasks
working the open backlog continuously, closing beads without the user asking.

### Execute (Investigation)

**Backlog counts** (`BEADS_DIR=~/beads/context-engine/.beads`):

| Status | Count |
|---|---|
| open | 61 |
| in_progress | 18 |
| closed | 60 |

**18 `in_progress` is itself a red flag** — these are beads `started` (an agent
picked them up) but never `closed`. That is the "spinning, not progressing"
signature: sessions begin work, stall or die, and leave the bead claimed.

**Who is closing beads — automated or manual?** Of the most recent closures
(`bd list --status closed --json`):
- All carry `created_by: vbonnet` and detailed, human-grade `close_reason`s
  referencing **manual deep work**: "Fixed in dotfiles PR #18 (merged …) +
  deployed live", strict `REVIEW.md` security/governance/LLM-judge gates, etc.
- **34 of the closed beads closed on 2026-06-11** — clustered around the user's
  hands-on session, not the 3-hourly cron marks.
- Searching every `close_reason` for "burndown" / "automated" / "scheduled"
  returns essentially nothing — only `ce-qgf` ("delivered via PR #179"). The
  automated loop has closed **~0** beads.

**Are the loop sessions making progress, or spinning?** The scheduled-task
state file (`.../scheduled-tasks.json`) records the answer directly:

```json
"recordedSkips": { "bead-burndown-loop": [
  { "at": 2026-06-11 15:18, "reason": "per_task_limit" },
  ... 30 skips, one per minute 15:18–15:29 ...
] }
```

**30 consecutive `per_task_limit` skips.** The scheduler tries to fire, hits a
per-task concurrency/resource limit, and skips — minute after minute. The loop
is effectively *not running*, and when it does run its SKILL caps itself at "2
concurrent tasks, and do nothing if any burndown session is still active" — so a
single stuck (`in_progress`) session wedges the whole loop.

### Audit

**What's blocking automated burndown from being effective:**

1. **`per_task_limit` starvation.** The scheduler is hitting a per-task limit
   and skipping ~every run (30 skips logged). Burndown almost never actually
   launches.
2. **Self-throttling SKILL.** "Do NOT start tasks if existing burndown sessions
   are still actively running" + "max 2 concurrent" means **one stalled
   `in_progress` session blocks all new work** — and we have 18 stalled beads.
   There is no health check, no restart of a stuck session, no concurrency
   *maintenance* — only a fire-and-forget every 3h.
3. **Wrong workspace (shared with Incident 1).** Tasks rooted in `~/src` either
   trip the read-only guard or corrupt the golden tree, so a launched task can
   die early — manufacturing more `in_progress` orphans.
4. **No verification loop.** Nothing reconciles `in_progress` beads against live
   sessions, so dead sessions never release their bead and never get retried.
5. **Cron cadence vs. continuous intent.** A 3-hourly cron that no-ops if a prior
   task is "active" is structurally incapable of *maintaining* N concurrent
   workers; it is a periodic poke, not an orchestrator.

**Recurring pattern?** Yes — this is the same class as the stranded-worktree and
stuck-task retros (`memory/dear-agent-retro-followthrough.md`,
`2026-05-13-enforcement-rules.md`): fire-and-forget automation with no
health/restart loop leaves orphaned state that compounds.

### Retro

**Root cause.** The burndown design is a **periodic, self-throttling cron poke**,
not a **concurrency-maintaining orchestrator**. It (a) gets starved by
`per_task_limit`, (b) wedges on a single stuck `in_progress` session, (c) is
rooted in the wrong (golden) workspace, and (d) has no health-check/restart/
reconcile loop — so stalled work never recovers and the user fills the gap by
hand.

**Proposed fixes** — detailed design in
[`docs/design/burndown-overseer.md`](../design/burndown-overseer.md):

1. **A VROOM Overseer (CRO-role) loop** that *maintains* target concurrency
   instead of poking: every tick it (i) lists live burndown sessions, (ii)
   reconciles `in_progress` beads against them, (iii) releases beads whose
   session is dead, (iv) restarts/refills up to the target (default 3), and
   (v) writes its decisions to the VROOM decision trail.
2. **Repoint workspace to worktrees** (shared with Incident 1, `BD-SRC-2`).
3. **Raise/diagnose `per_task_limit`** so the scheduler can actually launch
   (`BD-BURN-2`).
4. **Stuck-session detection:** a bead `in_progress` with no live session and no
   commit in N minutes is reclaimed to `open` and retried (`BD-BURN-3`).

**Decision: scheduled task is *not* sufficient — use a longer-running
orchestrator.** The cron stays only as a **watchdog** that ensures the Overseer
process is alive (restart-if-dead); the Overseer itself is the continuous loop
that holds concurrency. See the design doc for the supervisor pattern and why a
3-hourly cron cannot maintain a concurrency target.

**Beads filed:** `BD-BURN-1` (Overseer loop), `BD-BURN-2` (`per_task_limit`),
`BD-BURN-3` (stuck-session reclaim), `BD-BURN-4` (decision-trail wiring).

---

## The Fix — `src-recovery` (shipped in this PR)

- `cmd/src-recovery/main.go` + `internal/safesrc/recover.go` (+ tests) — a Go
  atomic wrapper (CLAUDE.md principle 9), mirroring `cmd/safe-push`.
- **Takes only a repo path** (plus `--dry-run`/`--timeout`); **no pass-through
  git arguments**, so there is no verb or flag an agent can smuggle in. Every
  git verb is a compile-time literal gated by an allowlist (`internal/safesrc`
  `allowedVerbs`); `commit`, `reset`, `push`, `branch`, `merge`, `rebase`,
  `clean`, `rm`, `fetch` are all rejected, enforced by test.
- **Validates** the path resolves (symlinks included) to a repo strictly under
  `~/src/`, rejecting `~/src` itself, `~/worktrees/*`, `../`-escapes and
  symlink-escapes (all tested).
- **Sequence:** refuse if conflicted → `stash --include-untracked` if dirty →
  `checkout <default-branch>` → `pull --ff-only` (time-bounded,
  `GIT_TERMINAL_PROMPT=0`). Nothing is ever discarded; stashed work is
  recoverable with `git -C <repo> stash pop`.
- **Audit trail:** appends a line per step to
  `~/.local/state/dear-agent/src-recovery.log` and echoes to stderr.
- **Verified live:** `src-recovery --dry-run ~/src/dear-agent` correctly refuses
  the current half-merged tree with hand-resolution guidance — proving the
  conflict guard works on the real incident.

**Allow-list step (chezmoi, sensitive — separate gated change):** add
`"Bash(src-recovery *)"` to `dot_claude/private_settings.json.tmpl` next to
`chezmoi-deploy` and `safe-push`. Per `memory/dotfiles-review-gate-workflow.md`
this touches agent permissions and goes through the strict `REVIEW.md` gate;
tracked as `ce-ueb7`'s follow-up rather than done inline here.

---

## Beads filed (real IDs)

The logical `BD-SRC-*` / `BD-BURN-*` names used above map to these
`context-engine` beads (`~/beads/context-engine`):

| Logical | Bead | P | Title |
|---|---|---|---|
| BD-SRC-1 | `ce-ueb7` | P0 | src-recovery wrapper (delivered; allow-list follow-up) |
| BD-SRC-2 | `ce-95yt` | P0 | Repoint bead-burndown-loop SKILL off ~/src |
| BD-SRC-3 | `ce-rrry` | P0 | Repoint youtube-channel-poll SKILL off ~/src |
| BD-SRC-4 | `ce-zjyu` | P0 | Close the `cd ~/src && git` write-guard bypass |
| BD-SRC-5 | `ce-qn81` | P0 | Recover the two golden checkouts via src-recovery |
| BD-SRC-6 | `ce-qfax` | P2 | poll-youtube-channels.py → Go |
| BD-BURN-1 | `ce-x67w` | P1 | Burndown Overseer (VROOM CRO) concurrency loop |
| BD-BURN-2 | `ce-2p6n` | P1 | Diagnose/raise `per_task_limit` |
| BD-BURN-3 | `ce-63xo` | P1 | Stuck-session reclaim (in_progress → open) |
