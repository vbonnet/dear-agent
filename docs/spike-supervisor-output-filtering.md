# Spike: Supervisor /loop Output Filtering (ce-4m85)

**Status:** spike / recommendation
**Bead:** ce-4m85 · **Overlaps:** ce-b1tt (GitHub webhooks)
**Date:** 2026-06-22

## Problem

The supervisors (`vroom-orchestrator`, `meta-orch`, `overseer`) run a `/loop`
tick roughly every 90s. Each tick re-reads raw bash output — `gh pr list`,
test logs, merge status, peer heartbeats — straight into model context. Most
ticks are no-ops: nothing changed since the last poll, but the model still
pays to read and reason over the full dump.

Measured costs (this repo, 18 open PRs):

| Source | Bytes/tick | ~Tokens/tick | Notes |
|---|---|---|---|
| `gh pr list --json …,statusCheckRollup` | 72,223 | ~18,000 | `statusCheckRollup` dominates |
| `gh pr list` (raw text) | 2,233 | ~560 | default columns only |
| `trail.jsonl` (full) | 2.3 MB | ~575,000 | 6,839 lines; never read whole |
| heartbeat `*.json` | 21–85 each | <30 | tiny, fine as-is |

The trap is counterintuitive: the **structured JSON is 32× larger than the raw
text**, because `statusCheckRollup` expands every check on every PR. Pulling
the rich JSON every tick is the worst case.

At ~40 ticks/hour, reading the full PR JSON every tick is **~720K tokens/hour
per supervisor**, almost all of it re-reading unchanged state. With three
supervisors this is the single largest avoidable token sink in the mesh.

## Option A — Structured delta pre-filter

A small pre-pass script runs *before* each tick and emits only what changed
since the previous tick.

- Persist last-tick state in `~/.agm/vroom/state/` (does not exist yet —
  create it): `prs.json` (number → {mergeable, checks-rollup, merged}),
  `last-tick.json`.
- The pre-pass diffs current `gh pr list` against stored state and prints a
  compact delta: `3 PRs newly merged: #671 #672 #673 | 1 CI red: #675 | 14 unchanged`.
- The supervisor reads ~100–300 tokens of delta instead of 18K of JSON.

**Savings:** ~98% vs full JSON, ~80% vs raw text, on steady-state (mostly
no-op) ticks. Pure mechanical win, no new infra, works today.

**Cost:** one extra `gh` call per tick (already happening), plus a state file.
Still poll-based — a no-op tick still wakes the supervisor and runs the
pre-pass, just cheaply.

## Option B — Claude Code Monitor tool (push-based)

The Monitor tool is a real Claude Code capability: it runs a background script
and turns each stdout line into a chat event/notification. This inverts the
loop — the supervisor reacts to events instead of polling.

- A `tail -f`/`inotifywait -m` monitor on `~/.agm/vroom/trail.jsonl` emits one
  line per new trail entry; the supervisor wakes only on real change.
- A poll-loop monitor on `gh pr checks`/`gh pr list` (30s+ to respect rate
  limits) emits one line per *state transition*, not per poll.
- `persistent: true` keeps the watch alive for the session.

**Savings:** eliminates no-op wakeups entirely — the dominant waste. Filter
must cover failure signals (stale peer, CI red, crash), not just the happy
path, or silence masks a problem.

**Cost:** Monitor still polls GitHub under the hood (the real push solution is
ce-b1tt webhooks); local FS events are genuinely push. Events arrive
asynchronously, which the supervisor loop must be written to tolerate.

## Option C — Hybrid (recommended)

Split by source, because the two sources have different shapes:

- **Local FS (trail.jsonl, heartbeat/\*.json):** truly file-change driven →
  **Monitor** with `inotifywait`/`tail -f`. Wake the supervisor on real
  writes, not a timer.
- **GitHub PR/CI state:** rate-limited remote API → **Option A structured
  delta pre-filter**, cached in `~/.agm/vroom/state/`, delta-only output.
  When ce-b1tt lands, swap the polling pre-pass for webhook push without
  changing the supervisor-facing contract (still a compact delta line).

This gets the push benefit where it's free (local FS) and the cheap-delta
benefit where push isn't available yet (GitHub), with a clean migration path
to webhooks.

## Recommendation

**Adopt Option C.** Sequence the work so value lands incrementally:

1. **First, Option A delta pre-filter for `gh pr list`** — highest ROI, zero
   new infra, ships today, cuts the ~720K tokens/hour/supervisor PR sink by
   ~98% on no-op ticks.
2. **Then, Monitor on local FS** (trail.jsonl, heartbeats) to drop no-op
   wakeups for mesh-internal state.
3. **Later, fold in ce-b1tt webhooks** behind the same delta contract to make
   GitHub genuinely push-based.

Pure Monitor (Option B) alone is rejected: it still polls GitHub and forces an
event-driven rewrite of the loop without first capturing the easy delta win.

## Next actions

- **ce-4m85.1** — Implement Option A pre-pass: `gh pr list` delta against
  `~/.agm/vroom/state/prs.json`, emit compact delta line. (S)
- **ce-4m85.2** — Wire the delta pre-pass into the three supervisor `/loop`
  prompts so they read the delta, not raw JSON. (S)
- **ce-4m85.3** — Add a Monitor watch on `trail.jsonl` + `heartbeat/*.json`;
  supervisor wakes on FS events. (M)
- **ce-b1tt** (existing) — GitHub webhooks; swap behind the Option A delta
  contract once available. (M)
