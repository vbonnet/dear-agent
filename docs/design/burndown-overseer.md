# Design — Bead-Burndown Overseer (VROOM CRO supervisor)

- **Status:** Proposal
- **Date:** 2026-06-11
- **Source retro:** [`docs/retros/2026-06-11-src-violations-and-burndown.md`](../retros/2026-06-11-src-violations-and-burndown.md)
- **Grounds in:** `pkg/vroom/supervisor/{supervisor,overseer,loop,check}.go`,
  `pkg/vroom/decisiontrail`, `CONTEXT.md §"The three supervisors"`.

## Problem (from the audit)

The `bead-burndown-loop` scheduled task is a **periodic, self-throttling cron
poke**, not a concurrency-maintaining orchestrator. Observed failure modes:

- **Starvation:** 30 consecutive `per_task_limit` skips logged — the loop
  almost never launches.
- **Wedging:** the SKILL says "do nothing if any burndown session is still
  active" + "max 2 concurrent," so a **single stuck `in_progress` session blocks
  all new work**. There are 18 stalled `in_progress` beads right now.
- **No reconciliation:** nothing maps `in_progress` beads back to live sessions,
  so a dead session's bead is never released or retried.
- **Wrong workspace:** tasks rooted in `~/src/<repo>` (Incident 1) die early and
  manufacture more orphans.

Net effect: automated burndown has closed **~0** beads; the user fills the gap
by hand (34 of the recent closures were manual, same-day, deep-work closes).

## Why a cron can't fix this

A 3-hourly cron that no-ops when a prior task is "active" can only *poke*; it
cannot *maintain a concurrency target*. Maintaining "always 3 workers" requires
a loop that observes current concurrency every few minutes, reconciles dead
sessions, and refills — i.e. a **long-running supervisor**, exactly the VROOM
Overseer (CRO) role that already exists in `pkg/vroom/supervisor`.

> **Decision: the scheduled task is NOT sufficient.** Keep the cron only as a
> *watchdog* that restarts the Overseer process if it has died. The Overseer is
> the continuous concurrency-holding loop.

## What already exists (reuse, don't reinvent)

`pkg/vroom/supervisor` already ships the machinery:

- `Supervisor` interface = `Role()` + `Tick(ctx)`. The `Loop` driver calls
  `Tick` on an interval and records outcomes to the decision trail; a returned
  error does **not** abort the loop.
- `Overseer` is the **CRO-analogue** supervisor — "Owns: resource usage, **leak
  detection, session cleanup**." It drives a `ResourceProbe` seam and emits
  escalations to a `decisiontrail.Trail`. PR-1 is observation-only; the doc
  already anticipates "a `SessionReaper` adapter that invokes `agm worktree
  sweep` / `agm session gc`."
- `CheckSkill` / `LoopStatus` give heartbeat-based health checks between peers.

Burndown-concurrency maintenance is a natural **second responsibility of the
Overseer**: same role (keep the system healthy and resources working), same
trail, same Tick cadence. We add one seam, not a new supervisor.

## Proposed design

### New seam: `BurndownController`

```go
// BurndownController is the seam the Overseer drives to keep N burndown
// workers alive. Mirrors ResourceProbe: the Overseer owns policy; the
// controller owns the side effects (listing/spawning/reclaiming).
type BurndownController interface {
    // Live reports the burndown worker sessions the controller believes are
    // actively making progress (heartbeat/commit within the freshness window).
    Live(ctx context.Context) ([]BurndownSession, error)

    // Reconcile maps in_progress beads to Live sessions and returns beads whose
    // owning session is dead (no live session, no commit within staleAfter).
    // These are candidates to reclaim to `open` and retry.
    Reconcile(ctx context.Context, staleAfter time.Duration) ([]StaleBead, error)

    // Reclaim moves a stale bead back to `open` so it can be re-dispatched.
    Reclaim(ctx context.Context, beadID string) error

    // Spawn starts one burndown worker against the given workspace + model,
    // returning the new session id. The workspace MUST be a ~/worktrees path
    // (never ~/src) — enforced by construction in the adapter.
    Spawn(ctx context.Context, spec WorkerSpec) (sessionID string, err error)
}

type BurndownSession struct {
    SessionID   string
    Model       string    // e.g. claude-opus-4-8
    BeadID      string    // bead currently claimed, if any
    LastHeartbeat time.Time
}

type StaleBead struct {
    BeadID    string
    SessionID string    // the dead session that had claimed it
    Reason    string    // "no-live-session" | "no-commit-within-window"
}

type WorkerSpec struct {
    Workspace string   // ~/worktrees/dear-agent-burndown-<n>  (asserted under ~/worktrees)
    Model     string
    Priority  string   // "P0" | "P1" | "P2"
}
```

### Overseer policy (extends the existing `Overseer.Tick`)

Add a `BurndownPolicy` to the Overseer alongside `EscalationThreshold`:

```go
type BurndownPolicy struct {
    Target     int           // desired concurrent workers (default 3)
    StaleAfter time.Duration // a claimed bead with no progress this long is stale (default 20m)
    Models     []string      // round-robin/assignment, e.g. [opus, opus, sonnet]
}
```

Each `Tick` (reusing the existing CRO Tick, after the resource snapshot):

1. **Observe.** `live := controller.Live(ctx)`.
2. **Reconcile.** `stale := controller.Reconcile(ctx, policy.StaleAfter)`; for
   each, `controller.Reclaim(beadID)` and emit a `decisiontrail` event
   (`burndown.reclaim`, with bead id + reason). This is the missing
   in_progress→open recovery loop.
3. **Maintain concurrency.** `deficit := policy.Target - len(liveAfterReclaim)`.
   If `deficit > 0` **and** the latest `ResourceSnapshot` is under threshold
   (don't spawn into a disk/mem/CPU escalation), `Spawn` `deficit` workers
   against fresh `~/worktrees/dear-agent-burndown-<n>` checkouts. Emit
   `burndown.spawn` per worker.
4. **Backlog guard.** If `bd list --status open` is empty, spawn nothing and
   emit `burndown.idle` ("backlog empty"). Never inflate work.
5. **Record.** Every decision (reclaim/spawn/idle/blocked-on-resources) goes to
   the `decisiontrail.Trail` — the append-only audit the cron never had.

Resource escalation is **already** handled by the existing Tick; burndown spawn
simply *respects* it (don't add workers when the box is saturated — which is
also what was producing `per_task_limit` skips).

### Health: keep the Overseer itself alive

- The Overseer runs under the existing `Loop` driver (heartbeat published each
  iteration). A sibling `CheckSkill`/`HeartbeatCheckSkill` already lets peers
  detect a degraded supervisor.
- The **cron `bead-burndown-loop` is repurposed to a watchdog**: every 3h (or
  more often) it checks the Overseer's heartbeat file; if stale/missing, it
  restarts the Overseer process and exits. It no longer spawns workers itself —
  that removes the self-throttling logic that wedged on one stuck session.

### Adapters (follow-up PRs, mirroring the ResourceProbe rollout)

- `Live`/`Spawn` over **AGM**: `agm session list` + `agm session new`
  (`mcp__agm__agm_list_sessions`), workspace asserted under `~/worktrees`.
- `Reconcile`/`Reclaim` over **beads**: `bd list --status in_progress --json`,
  cross-ref session ids / last commit time, `bd update <id> --status open`.
- Reuse the anticipated `SessionReaper` (`agm worktree sweep` / `agm session
  gc`) so reclaimed workers don't leak worktrees (ties to
  `memory/dear-agent-worktree-stop-reaper.md`).

## Rollout (PR-sized, dark-factory rules)

1. **PR-A** — `BurndownController` interface + `InMemoryBurndownController` +
   `BurndownPolicy` + Overseer Tick extension + tests. Observation+in-mem only,
   no real side effects (mirrors the ResourceProbe PR-1 pattern). *(Bead
   `BD-BURN-1`.)*
2. **PR-B** — AGM/beads adapters behind the interface; workspace-under-worktrees
   assertion; wire to `decisiontrail`. *(Beads `BD-BURN-3`, `BD-BURN-4`.)*
3. **PR-C** — convert the cron SKILL to a watchdog; raise/diagnose
   `per_task_limit`; repoint any remaining workspace refs off `~/src`. *(Beads
   `BD-BURN-2`, `BD-SRC-2`.)*

## Open questions

- **`per_task_limit` source.** Is the limit the Desktop scheduler's own
  per-task concurrency cap, or a host RAM/CPU budget? `BD-BURN-2` is to locate
  and tune it; the Overseer's resource-escalation gate makes a *higher* limit
  safe (it self-throttles on real saturation instead of a blunt count).
- **Target vs. RAM.** Default `Target=3` assumes 3 concurrent Claude Code
  sessions fit the box; the resource snapshot gate lets the Overseer drop below
  target under pressure rather than thrash.
- **sql-server beads.** Per `memory/workitem-ledger-vs-beads.md`, ≥3 concurrent
  agents writing beads needs the Dolt sql-server (embedded concurrency
  corrupts). The Overseer maintaining 3 workers **requires** the sql-server be
  up — add that as a precondition the Overseer asserts before spawning.
