# Resource Lifecycle Manager — GC for Agent Sessions

**Status:** Design / RFC
**Date:** 2026-06-07
**Author:** drafted with Claude (Opus 4.8) for dear-agent
**Tracking:** Beads — "Resource lifecycle manager — GC for agent sessions" (P1; `dear-agent`, `infrastructure`, `architecture`)
**Scope:** design only. No implementation, no scope creep into adjacent cleanup work.

---

## 1. TL;DR

An agent session creates resources — file edits, branches, worktrees, child
processes, tmux sessions — and today there is no single model that says *who
owns each one* and *when it must be resolved*. We have several point solutions
(`agm worktree sweep`, the Stop-hook worktree reaper, `agm session gc`) that
each re-derive ownership and reapability with their own heuristics. They work,
but they overlap, drift apart, and cover only worktrees/branches — processes
and tmux sessions leak silently.

This document proposes a **Resource Lifecycle Manager (RLM)**: a single
ownership-and-reclamation model for all agent-created resources, framed like a
compiler's borrow checker (ownership is explicit and checked at boundaries) plus
a garbage collector (a periodic sweep reclaims what slipped through). It is
deliberately **not new infrastructure built from scratch** — it formalizes and
unifies what dear-agent already has:

- `Manifest.Resources` (per-session `Worktrees[]` / `Branches[]`) is already a
  reference-counting table. RLM extends it to processes and tmux sessions.
- `agm worktree sweep` is already mark-and-sweep with a three-legged safety
  oracle. RLM generalizes that oracle to every resource type.
- `pkg/vroom/decisiontrail` is already an append-only audit log. RLM writes
  reclamation events there.

**Recommended strategy: hybrid** — deterministic, RAII-style cleanup at session
boundaries (cheap, immediate, owner-driven) backed by a periodic mark-and-sweep
for leaked resources (the safety net for crashed or force-killed sessions). The
two GC strategies that dear-agent already runs are exactly these two halves; the
proposal is to make them one system with one ownership source of truth.

---

## 2. Problem statement

### 2.1 What goes wrong today

A session does work, then ends — cleanly, by crash, by `stop`, or by the user
walking away. Resources it created outlive it:

- **Uncommitted edits** sit in a worktree. The next session (or a sweep) can't
  tell whether they're abandoned WIP or in-flight work, so they're never reaped
  (correctly — but they accumulate).
- **Branches and worktrees** survive merge. The reaper exists (PR #120 Stop
  hook, PR #125 `agm worktree sweep`) but it has had to be re-fixed repeatedly
  (memory: "effective reaper was orphaned 4×") and a third parallel
  await/merge heuristic drifted onto `wtpolicy`.
- **Child processes** — a hung test, a `go run` server, a background `tail` —
  have *no* owner tracking at all. They leak until the host is rebooted or a
  human notices CPU burn.
- **tmux sessions** created mid-task (the agent spawns a helper pane) are
  likewise untracked outside the AGM-managed top-level session.

### 2.2 Why a unified model, not more point fixes

Each existing reaper re-implements three things: *what resources exist*, *who
owns them*, and *is it safe to reclaim*. Memory records that this triplication
has already produced silent reverts and drift:

- "#126 ships a 3rd parallel await/merge heuristic NOT on `wtpolicy` (drift
  materialized)."
- "merge-tree lies for split refactors… has TWICE silently reverted a security
  fix."

Three reapers with three oracles is three chances to disagree. The borrow-checker
framing forces the question to be answered **once**, at the point of creation,
and recorded — so reclamation is a lookup, not a re-derivation.

This is itself an instance of dear-agent Principle 3 ("broken thing? DEAR retro
→ new agent → scoped plan, never fix in-line") and Principle 9 ("atomic action
wrappers"): the safe sequencing of "resolve resource before session ends" should
live in code and at hook boundaries, not in agent discipline.

---

## 3. Core concept: ownership + reclamation

Two borrowed ideas, mapped onto sessions:

### 3.1 Borrow-checker analogy (ownership, checked at boundaries)

When a session creates a resource it becomes the **owner** and holds
responsibility for that resource's lifecycle. Ownership is:

- **Explicit** — recorded at creation time in the owning session's manifest, not
  inferred later from filesystem archaeology.
- **Singular** — exactly one live owner at a time (like Rust's move semantics).
  Two sessions can't both be responsible for the same worktree.
- **Checked at boundaries** — the session cannot cross a lifecycle boundary
  (end of turn, end of session, archive) while holding an unresolved resource of
  a kind that boundary governs. The hook is the borrow checker; it refuses to
  "compile" the transition until the resource is resolved.

A resource is **resolved** when it reaches a terminal state appropriate to its
type (committed/reverted, merged/cleaned/abandoned, killed/handed-off). An
unresolved resource at a boundary is an error the owner must fix — exactly like a
value that's still borrowed at the end of its scope.

### 3.2 GC analogy (reclamation, for what slips through)

Boundary checks are owner-cooperative: they assume the session is alive to run
its Stop/SessionEnd hook. Sessions die uncooperatively — OOM, `kill -9`, laptop
sleep, supervisor `stop`. For those, ownership records become **roots** for a
mark-and-sweep collector:

- **Mark:** enumerate live sessions (roots) and the resources they own.
- **Sweep:** any resource whose owning session is dead *and* which the safety
  oracle proves reclaimable gets reclaimed; everything else is kept and surfaced.

The collector is the backstop, not the primary path. Most resources are resolved
by their owner at a boundary; the sweep only catches leaks.

---

## 4. Resource taxonomy

Four resource kinds, each with a lifecycle rule, a resolution set, the boundary
that enforces it, and the safety check that gates reclamation.

| Kind | Lifecycle rule | Resolved states | Enforcing boundary | Safety oracle |
|------|----------------|-----------------|--------------------|---------------|
| **File edits** | committed or reverted by end of turn | committed; reverted/clean | **Stop** hook (end of turn) | `wtpolicy.Dirty` (same check the sweep uses for `ClassDirty`) |
| **Branches / worktrees** | cleaned up before archive | merged+removed; explicitly abandoned | **archive** guard | `wtpolicy.ProvablyMerged` (three-legged: ancestor + PR state + unpushed-commit guard) |
| **Processes / jobs** | killed unless handed off | exited; reparented/handed off | **SessionEnd** hook | child-of-session-pgid check; liveness probe |
| **tmux sessions** | cleaned up before archive | killed; handed off | **SessionEnd** hook + **archive** guard | created-by-this-session tag; no live attach |

### 4.1 File edits — "commit or revert"

**Rule:** by the end of a turn, the working tree the session is editing has no
uncommitted changes. Either the work is committed (Principle: "uncommitted work
is nonexistent work" — Agent Delegation Enforcement §1) or deliberately reverted.

**Why end-of-turn and not end-of-session:** the incremental-commit discipline
already mandated in CLAUDE.md wants commits at *sub-task boundaries*, and a turn
is the finest boundary a hook can see. Enforcing at turn granularity makes the
"commit early" rule self-checking rather than aspirational.

**Enforcement (Stop hook):** check the session's working dir(s) for uncommitted
changes. **Reuse `wtpolicy.Dirty`** (`agm/internal/ops/wtpolicy/wtpolicy.go:42`)
— the *same* dirty-check the worktree classifier already calls to assign
`ClassDirty`, and which is already shared with archive-ui. Do **not** add a fresh
`git status --porcelain` shell-out: that would be a parallel dirty oracle, the
exact drift this design exists to prevent. The "one oracle" constraint applies to
file edits too — the file-edit boundary check and the worktree `ClassDirty` verdict
are the same observation at two boundaries. If `Dirty` reports true (or `known`
is false → fail-safe block), the hook returns a non-zero exit with the message:

> You have uncommitted changes in `<path>`. Commit them (incremental WIP commits
> are fine and encouraged) or revert them before ending the turn. Uncommitted
> work does not survive a killed worker. To intentionally keep dirty work across
> turns, mark it: `agm resource hold <path> --reason "..."`.

**Escape hatch:** a legitimate "I'm mid-edit and want to think" case exists. The
`hold` marker (see §7.3) records an explicit, reasoned exception so the check
passes *and* the intent is auditable — rather than the agent learning to route
around a bare refusal (Principle 2).

### 4.2 Branches / worktrees — "merge and clean up, or explicitly abandon"

**Rule:** before a session is archived, every branch/worktree it owns is either
landed-and-removed or explicitly abandoned with a recorded reason.

**Enforcement (archive guard):** `ArchiveSession()`
(`agm/internal/ops/session_archive.go:44`) already runs pre-archive verification.
RLM adds a resource-clean check there: for each owned worktree/branch, run the
existing classifier (`agm/internal/ops/worktree_sweep.go` `Classification`). If
any resource is not in a terminal class and not flagged abandoned, block the
archive:

> Session `<id>` still owns worktree `<path>` (branch `<b>`, class=ORPHANED).
> Land it (merge + `agm worktree sweep --execute`), or abandon it explicitly:
> `agm resource abandon <path> --reason "superseded by #NNN"`. Archive is
> blocked until every owned worktree/branch is resolved.

**Reuse, do not reinvent:** the safety oracle is the *existing* three-legged test,
already consolidated behind `wtpolicy.ProvablyMerged`
(`agm/internal/ops/wtpolicy/wtpolicy.go:83`), which composes `IsAncestor`,
`PRState`, and `HasUnpushedCommits` (the `SweepDeps`/`GitProbe` interface). That
`wtpolicy` package is the concrete home of the "one oracle" — it already unifies
the dirty-check (`Dirty`) and the merge verdict (`ProvablyMerged`) behind one
interface shared by sweep and archive-ui. RLM does not add a fourth heuristic —
the documented failure mode is *too many* heuristics drifting apart (the prior
drift was a parallel await/merge heuristic that escaped `wtpolicy`; the fix is to
route *everything* back through it). The classifier's allowlist stance (reap only
`ClassMerged`; keep `ClassUnknown`/`ClassOrphaned`) is preserved verbatim.

### 4.3 Processes / jobs — "kill unless handed off"

**Rule:** child processes a session spawned (including stuck/hung ones — the
hanging test is the canonical case) are terminated at SessionEnd, unless
management has been explicitly handed to another owner.

**Ownership tracking:** processes are the gap today. The cheapest reliable owner
signal is the **process group**: a session runs in a known pgid (the tmux pane /
shell it drives), and `Bash`-tool children inherit it. SessionEnd enumerates
live processes whose pgid descends from the session's root pgid.

**Enforcement (SessionEnd hook):**
1. Enumerate owned live PIDs (pgid descent + recorded spawns from the
   PostToolUse Bash tracker, see §7.2).
2. For each: if it's flagged handed-off (§6), skip. Otherwise terminate
   gracefully (`SIGTERM`, grace period, then `SIGKILL`) and record the
   reclamation in the decision trail.
3. Stuck jobs are not special-cased — a hung test is just a live owned process;
   the same TERM/KILL path reclaims it.

**Safety:** never kill outside the session's own pgid subtree. The
`macos-env-gaps` and classifier memory both warn that killing foreign processes
is denied and dangerous — the pgid-descent constraint is what keeps reclamation
inside the session's own blast radius.

### 4.4 tmux sessions — "clean up before archive"

**Rule:** tmux sessions a session created (beyond the AGM-managed top-level one)
are killed at SessionEnd / before archive, unless handed off.

**Ownership tracking:** AGM already manages top-level tmux sessions. For
*agent-created* helper sessions, tag them at creation with the owning session id
(tmux supports per-session user options / a naming convention like
`agm-child-<sessionID>-<n>`). The SessionEnd hook lists tmux sessions carrying
this session's tag and not currently attached by a human.

**Enforcement:** kill tagged, unattached, owned tmux sessions at SessionEnd; the
archive guard double-checks none remain (defense in depth — SessionEnd may not
fire on a hard crash, so archive re-verifies).

---

## 5. GC strategy evaluation

The four candidate strategies, scored against dear-agent's actual constraints
(sessions die uncooperatively; reclamation must be fail-safe; we already run two
of these).

### 5.1 Reference counting

Each resource records its owning session; when the session ends, owned resources
are resolved. **This is what `Manifest.Resources` already is** — a per-session
table of `Worktrees[]`/`Branches[]` with creation timestamps.

- **Pro:** deterministic, immediate, cheap. Reclamation is a table lookup, not a
  filesystem scan. Ownership is unambiguous.
- **Con:** relies on the decrement actually running. A `kill -9`'d session never
  runs its "release" — the refcount leaks. Classic refcounting can't collect
  cycles; we have no cycles, but we do have the equivalent: orphaned counts from
  crashes.
- **Verdict:** necessary but not sufficient. It is the *ownership substrate* for
  everything else, but cannot be the only mechanism.

### 5.2 Mark and sweep

Periodically enumerate all resources, mark those reachable from a live session,
sweep the rest. **This is what `agm worktree sweep` already is.**

- **Pro:** catches leaks from crashed sessions — exactly the case refcounting
  misses. Stateless: correctness doesn't depend on any earlier event having
  fired.
- **Con:** latency (a leaked process burns CPU until the next sweep); cost (full
  scan); and it needs a rock-solid liveness oracle, because "no live owner" must
  not misfire on a session that's merely paused. The existing sweep's fail-safe
  default (`ClassUnknown` → keep) is the right bias.
- **Verdict:** necessary as the backstop. Too slow/expensive to be the primary
  path, especially for processes where leak cost is continuous.

### 5.3 Generational

Check recently created resources more often than old ones, on the hypothesis that
most resources die young.

- **Pro:** matches the empirical lifetime distribution — a worktree created this
  turn and merged next turn is short-lived; a long-running dev server is not.
  Concentrates sweep effort where churn is.
- **Con:** real complexity (per-resource age tracking, promotion between
  generations, tuning the frequency knobs) for a fleet that is small enough that
  a flat sweep is cheap. The generational hypothesis is true here, but the payoff
  is an optimization we don't yet need.
- **Verdict:** not now. Revisit only if sweep cost becomes material. Capture the
  age data (`CreatedAt` already exists on resources) so a future generational
  pass is possible without a migration.

### 5.4 Hybrid (RAII/defer + periodic sweep) — **recommended**

Deterministic owner-driven cleanup at session boundaries (the refcount
decrement, run as a `defer`/RAII finalizer in the Stop/SessionEnd/archive hooks)
**plus** a periodic mark-and-sweep for whatever slipped through.

- **Pro:** the boundary path handles the common case immediately and cheaply
  (most sessions end cooperatively); the sweep handles the uncooperative tail.
  Each covers the other's blind spot: refcounting misses crashes, sweep is slow
  for live leaks — together they're both fast *and* complete.
- **Con:** two code paths that must agree on the ownership model and the safety
  oracle. **This is precisely the failure we've already lived** (three reapers,
  three oracles, silent drift). The hybrid is only safe if both paths read **one
  ownership table** and call **one oracle**.
- **Verdict:** **recommended**, with the hard constraint that it unifies the
  existing pieces rather than adding a parallel one. dear-agent already runs the
  two halves (boundary reaper + periodic sweep); they are simply not yet a single
  system with a single source of truth.

**Decision:** Hybrid. Refcounting (`Manifest.Resources`, extended to all four
kinds) is the source of truth; RAII-style boundary hooks are the primary
reclamation path; one generalized mark-and-sweep is the backstop; generational is
deferred but its inputs are recorded.

---

## 6. Ownership transfer

> *Q2: session A creates a worktree, session B takes over.*

Transfer is a **move**, not a copy: after transfer A no longer owns the resource
and is not blocked by it at its boundaries; B now owns it and is.

**Mechanism:** `agm resource transfer <resource> --to <sessionID> [--reason ...]`

1. Validate B is a live session and the resource is currently owned by the caller
   (A). Reject if the resource is unowned or owned by a third party (no stealing
   without a force flag + audit record).
2. Atomically update the ownership record: remove from A's `Manifest.Resources`,
   add to B's, stamp `TransferredFrom`, `TransferredAt`, `Reason`.
3. Append a `resource.transfer` record to the decision trail (who → whom, what,
   why).

**Implicit transfer for parent/child sessions:** `Manifest` already has
`ParentSessionID`. When a child (execution) session is spawned to work in a
parent's worktree, ownership stays with the parent and the child *borrows* (a
read-style borrow): the child's SessionEnd does **not** reap a resource it
doesn't own. This avoids the bug where a short-lived child session ending would
reap its parent's still-active worktree.

**Process hand-off** (the "unless management is handed off" clause): a session
that intentionally leaves a long-running server for another session calls
`agm resource transfer <pid> --to <sessionID>` (or `--detach` to formally
orphan it to no owner, see §7.4). Without an explicit hand-off, SessionEnd kills
it — the default is reclaim, hand-off is the opt-in.

---

## 7. Ownership tracking — design detail

> *Q1: session metadata? a manifest file? git notes?*

**Decision: session metadata (the AGM manifest, persisted via the Dolt-backed
`Store`), not git notes, not a separate manifest file.**

### 7.1 Where ownership lives

The owning record is `Manifest.Resources` (a `ResourceManifest`, currently
`Worktrees[]`/`Branches[]`) in the AGM session manifest (`Manifest` struct at
`agm/internal/manifest/manifest.go:13`, `ResourceManifest` at `:147`). The
manifest is persisted through the `Store` interface
(`agm/internal/manifest/store.go`) whose **canonical implementation is Dolt** —
the *same* store that already holds the `agm_worktrees` lifecycle table. Rationale:

- It already exists and already tracks worktrees/branches with timestamps.
- It's queryable transactionally alongside session lifecycle state — the sweep's
  "is the owner alive?" question is a join in the same Dolt DB. Critically,
  `Manifest.Resources` and `agm_worktrees` are **two tables in one store**, not
  two databases, so reconciling them is a within-store concern, not a cross-store
  sync (see §12, §13).
- **Rejected: git notes.** Notes only describe git objects (commits/branches) —
  they can't own a PID or a tmux session, and they don't help for the uncommitted
  case. A model that covers all four resource kinds can't be git-anchored.
- **Rejected: standalone manifest file.** A file on disk is a second source of
  truth that drifts from the DB — re-creating the exact triplication problem
  this design exists to kill.

### 7.2 How resources get recorded at creation

- **Worktrees/branches:** the existing PostToolUse `posttool-worktree-tracker`
  hook already records git worktree lifecycle (to the Dolt `agm_worktrees`
  table). RLM routes that signal into the canonical `Manifest.Resources` (or
  reconciles the two stores — see Open Questions §12).
- **Processes:** extend the PostToolUse Bash tracker to record long-lived /
  backgrounded children (PID, pgid, command, started-at) under the session.
  Short foreground commands that already exited need no record.
- **tmux:** record at the point the agent creates a tagged session.
- **File edits:** *not* individually recorded — "uncommitted state in working
  dir X" is computed on demand via `git status`. There's nothing to track; the
  working tree is its own ledger.

### 7.3 Holding dirty work across a turn

`agm resource hold <path> --reason "..."` writes a turn-scoped exception so the
Stop hook's file-edit check passes. The hold is recorded with a reason and
auto-expires (it must be re-asserted next turn, so a forgotten hold doesn't
become permanent permission to leak). This is the positive-guidance escape hatch
(Principle 2): the rule teaches the safe path *and* offers an auditable override.

### 7.4 Detach (orphan to no owner)

`agm resource detach <resource>` formally sets the resource to *unowned*. Used
when a resource should outlive all sessions (a dev server the user wants running
indefinitely). A detached resource is skipped by SessionEnd reaping but **is**
surfaced by the periodic sweep as "intentionally orphaned" so it never becomes
invisible. Detach differs from abandon (§8): detach = "keep it running, nobody
owns it"; abandon = "stop tracking it, it's dead to me."

---

## 8. The "explicitly abandoned" case

> *Q3: the user decides to leave a branch open intentionally.*

Abandonment is a **first-class resolved state**, not a failure to clean up. This
mirrors the repo's `graceful-exit` guardrail philosophy ("nothing fits" is a
valid outcome) and Principle 2 (a rule must offer a sanctioned escape, or agents
route around it).

`agm resource abandon <resource> --reason "<why>"`:

- Marks the resource resolved-by-abandonment in the ownership record, with a
  mandatory reason and timestamp.
- **Releases the boundary block:** an abandoned branch no longer blocks archive.
- Appends a `resource.abandon` record to the decision trail — so abandonment is
  *visible and reviewable*, never silent. The Audit phase (§9) can list abandoned
  resources and a human can later reclaim or resurrect them.
- Crucially, abandon does **not** delete. An abandoned branch stays on disk/remote
  for human disposal. The point is to stop *blocking the owner* while keeping the
  artifact and its rationale.

The reason is mandatory by construction (the command rejects an empty reason).
This is the difference between "the agent gave up and left a mess" and "the owner
made a recorded decision to leave this open" — the latter is legitimate, the
former is the leak this whole design targets.

---

## 9. Hook selection — which boundary enforces what

> *Q4: end of turn vs end of session vs archive.*

Three boundaries, three scopes, ordered by how much they can assume about the
session being alive.

| Boundary | Hook event | Fires when | Governs | Can assume session alive? |
|----------|-----------|------------|---------|---------------------------|
| **End of turn** | `Stop` | agent finishes a response turn | **file edits** (commit/revert) | yes — fully interactive |
| **End of session** | `SessionEnd` | session is ending (clean) | **processes, tmux** | mostly — best-effort, may be skipped on crash |
| **Archive** | archive guard in `ArchiveSession()` | session transitions to `archived` | **branches/worktrees**; re-verify processes/tmux | no — session is already gone |

**Rationale for the mapping:**

- **File edits → Stop (end of turn).** Highest frequency, cheapest to check,
  matches the incremental-commit mandate. Checking only at session end would let
  a 90-minute multi-turn session accumulate uncommitted work that a crash erases
  — the exact loss CLAUDE.md §1 forbids.
- **Processes/tmux → SessionEnd.** These should not block individual turns (a
  dev server is *meant* to live across turns within a session). They only need to
  be gone when the session itself ends.
- **Branches/worktrees → archive.** A branch is meant to live across the whole
  session and possibly past SessionEnd (work-in-review). The true "must be
  resolved" point is archive — the moment the session is being put away for good.
- **Defense in depth:** because SessionEnd is **not guaranteed** on a hard crash,
  the archive guard *re-verifies* processes and tmux too. And the periodic sweep
  (the GC backstop) catches anything that escaped both — a session that crashed
  and was never archived.

**Boundary check = borrow-check; sweep = GC.** The hooks are the deterministic
RAII path (§5.4); the sweep is the collector for what the hooks couldn't run.

---

## 10. Integration with the existing agm session model

> *Q5.*

RLM is an extension of existing components, not a new subsystem. Concrete touch
points:

| RLM concept | Existing component to extend | Path |
|-------------|------------------------------|------|
| Ownership table | `ResourceManifest` under `Manifest.Resources` (add `Processes[]`, `TmuxSessions[]`) | `agm/internal/manifest/manifest.go:13` (Manifest), `:147` (ResourceManifest) |
| Session liveness root (for mark phase) | `Manifest.Lifecycle` / `State`; session DB | `agm/internal/db/sessions.go` |
| Archive guard | `ArchiveSession()` pre-archive verification | `agm/internal/ops/session_archive.go:44` |
| Reclamation safety oracle | `wtpolicy` (`Dirty` + `ProvablyMerged`) + `Classification`; reuse verbatim, do not fork | `agm/internal/ops/wtpolicy/wtpolicy.go:42,83`, `agm/internal/ops/worktree_sweep.go:15` |
| Periodic sweep | `agm worktree sweep` (generalize to all kinds) | `agm/cmd/agm/worktree_sweep.go` |
| Boundary cleanup at session GC | `agm session gc` | `agm/cmd/agm/session_gc.go:20` |
| Resource recording at creation | PostToolUse hooks (`posttool-worktree-tracker`, extend Bash tracker) | `agm/cmd/agm-hooks/` |
| Audit log | `pkg/vroom/decisiontrail` (`Record{EventID,Role,Kind,Payload}`) | `pkg/vroom/decisiontrail/trail.go:36` |
| Protected-role exemption | `agm session gc` already exempts orchestrator/meta-orchestrator/overseer | `agm/cmd/agm/session_gc.go` |

**Notable alignment:** `agm session gc` already archives stopped sessions
inactive >24h while protecting supervisor roles. RLM's reclamation is the same
operation viewed per-resource instead of per-session — so the natural home for
the generalized sweep is alongside `session gc`, sharing its protected-role
logic. Supervisor sessions (which legitimately hold long-lived resources) must be
exempt from aggressive reaping for the same reason they're exempt from `gc`.

**CLI surface (proposed, design-level only):**

```
agm resource list [--session <id>]      # what does this session own?
agm resource transfer <res> --to <id>   # move ownership (§6)
agm resource hold <path> --reason ...    # turn-scoped dirty-work exception (§7.3)
agm resource abandon <res> --reason ...  # first-class resolved-by-abandon (§8)
agm resource detach <res>                # orphan to no owner (§7.4)
agm resource sweep [--execute]           # generalized mark-and-sweep (dry-run default)
```

`--execute` is **opt-in** and dry-run is the default, matching the existing
`agm worktree sweep` stance. Reaping (a destructive, all-or-nothing chain)
is exactly the kind of action Principle 9 says to wrap: the sweep binary owns the
ordering and the safety gate; the raw destructive commands stay denied.

---

## 11. DEAR mapping

The design lands as a Define → Enforce → Audit → Refine loop, which is how every
durable rule in this repo is structured.

- **Define** — CLAUDE.md gains a "Resource Ownership" section: a session owns
  what it creates; resources have terminal states; ownership is singular and
  transferable. `.dear-agent.yml` can carry per-repo resource policy (e.g.
  default process grace period). The taxonomy table (§4) is the normative
  definition.
- **Enforce** — Stop hook (file edits), SessionEnd hook (processes + tmux),
  archive guard (branches/worktrees + re-verify). Each returns *positive
  guidance* (Principle 2): names the resource, states the resolved options
  (commit/revert · merge/abandon · kill/handoff), and points at the command.
- **Audit** — the periodic generalized sweep, plus a decision-trail query: every
  reclamation, transfer, abandon, and detach is a `Record` in
  `pkg/vroom/decisiontrail`. "What did we reap this week, and what's intentionally
  orphaned?" is a trail scan. Leaked-resource counts are the health metric.
- **Refine** — sweep/audit findings feed a retro (`docs/retros/`). If a class of
  resource keeps leaking (e.g. a tool that spawns untracked children), the retro
  produces a scoped action item to improve the *definition* (track that spawn) or
  *enforcement* (a new boundary check) — never an in-line patch (Principle 3).

This closes the loop: the same audit that reclaims leaks also produces the data
that tightens the rules, so the leak class shrinks over time instead of being
whack-a-moled per incident.

---

## 12. Edge cases & failure modes

- **Hard crash before any hook fires.** Refcount never decremented, SessionEnd
  skipped. → Caught by the periodic sweep (the entire reason the hybrid keeps a
  GC backstop). The ownership record is the root; a dead owner makes its
  resources sweep candidates.
- **Paused/compacting session mistaken for dead.** A session mid-compaction must
  not have its resources reaped. → Liveness oracle keys on `Manifest.State` /
  `Lifecycle`, not just process presence; `ClassUnknown`/`ClassActive` →
  fail-safe keep. Bias is always "keep and surface," never "reap on doubt."
- **Transfer race (A transfers to B as B is ending).** → Transfer is a
  transactional ownership update; if B ends before committing, ownership stays
  with A (the move didn't complete). No window where the resource is unowned.
- **Two tables of truth (`agm_worktrees` vs `Manifest.Resources`).** The worktree
  tracker writes the `agm_worktrees` table; ownership lives in `Manifest.Resources`.
  Both are in the **same Dolt store** (the manifest `Store`'s canonical backend is
  Dolt, and `agm_worktrees` is a Dolt table — verified: `agm/internal/dolt/`,
  `agm/internal/manifest/store.go`). So this is *not* a cross-store sync problem —
  it's two tables in one transactional DB that must agree. Still must be reconciled
  or one made authoritative, or we reproduce the drift problem inside the fix. →
  Open question §13; recommended: `Manifest.Resources` is authoritative, the
  `agm_worktrees` table becomes a denormalized audit/event view populated from it.
- **Killing a process that re-parented (daemonized).** A child that `setsid`'d
  out of the session pgid escapes the descent check. → Record PID + pgid at
  spawn; on SessionEnd, check the *recorded* PID's liveness directly, not only
  live pgid descent. Accept that a truly daemonized process is "intentionally
  detached" and surface it rather than hunting it.
- **Supervisor sessions hold long-lived resources legitimately.** → Protected
  roles exempt from reaping (reuse `session gc`'s role list).
- **`make preflight` / CI flakes vs hung-test detection.** A hung test is a leaked
  process to reclaim, but a *slow* test is not. → SessionEnd reaps only on
  session end, not on a timeout heuristic; we don't try to distinguish "hung"
  from "slow" mid-run — ending the session is the unambiguous signal.

---

## 13. Open questions

1. **Table reconciliation (within Dolt):** `Manifest.Resources` and the
   `agm_worktrees` table are both in the one Dolt store, so this is a same-store,
   transactional question, not a cross-database one. Make `Manifest.Resources`
   authoritative and demote `agm_worktrees` to an audit/event view derived from
   it, or keep both and add a reconciler? (Recommend the former — single writer,
   one source of truth.) The single-store fact removes the hardest objection
   (two-phase sync), so this is mostly a schema-ownership decision.
2. **Process ownership reliability:** is pgid-descent + recorded-PID enough on
   macOS, or do we need a more robust tagging mechanism (env var stamped into
   children, e.g. `AGM_OWNER_SESSION`)?
3. **tmux tagging convention:** per-session user option vs naming convention vs
   both — which survives tmux server restarts?
4. **Hold/abandon authority:** can any agent `abandon`, or does abandoning a
   branch with unpushed commits require user/supervisor sign-off (it's
   data-loss-adjacent)? Likely route through VROOM for the unpushed case.
5. **Sweep cadence:** on-Stop trigger (PR #189 added a post-merge sweep trigger)
   vs cron vs both. Generational tuning deferred (§5.3) but cadence isn't.
6. **File-edit scope:** a session may touch multiple worktrees in a turn — does
   the Stop check cover all of them or only the "primary" one?

---

## 14. Explicitly out of scope

To hold the line on Principle 1 (no scope creep), this design does **not**:

- Implement anything — this is design only.
- Re-litigate or rewrite the existing worktree reaper / `agm worktree sweep`
  internals beyond *generalizing* their oracle. The three-legged safety check is
  reused as-is.
- Add a fourth merge/await heuristic. The documented drift problem forbids it.
- Touch the supervisor mesh's own resource handling beyond honoring its
  protected-role exemptions.
- Define resource types beyond the four specified (no network ports, no temp
  files, no cloud resources — those can be future kinds if the model proves out).

---

## 15. Appendix — strategy decision summary

| Strategy | Role in final design | Status |
|----------|---------------------|--------|
| Reference counting | Ownership source of truth (`Manifest.Resources`, extended) | **Adopted** as substrate |
| Mark and sweep | Backstop collector for crashed/uncooperative sessions | **Adopted** as backstop |
| Generational | Sweep-cost optimization; inputs (`CreatedAt`) recorded | **Deferred** |
| Hybrid (RAII + sweep) | Overall architecture: boundary hooks primary, sweep backstop, one ownership table, one oracle | **Recommended** |

The single most important constraint, learned from prior drift: **one ownership
table, one safety oracle, two reclamation paths.** The hybrid only works if the
deterministic boundary path and the periodic sweep are the same system reading
the same truth — not a third reaper alongside the two we already have.
