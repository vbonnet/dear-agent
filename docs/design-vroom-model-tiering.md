# Design Spike: VROOM Model Tiering (Opus for Supervisors, Sonnet/Haiku for Workers)

**Status**: Spike (design only — implementation tracked separately as W0)
**Date**: 2026-06-20
**Bead**: ce-th8k
**Strategic theme**: token-economics (ties into token-economics P0)
**Source**: fusion-agent research (Abacus AI swarm architecture)
**Grounding**: [ADR-002](adr/ADR-002-vroom-execution-architecture.md) (the
three-supervisor mesh), [/CONTEXT.md](../CONTEXT.md) (role vocabulary).

## Problem

Every VROOM session — supervisors and workers alike — runs on `opus-200k`.
Opus is the right tool for the mesh's reasoning hubs (meta-orchestrator,
orchestrator, overseer), where a wrong dispatch or a missed gate cascades
across the whole swarm. It is overkill for a worker that resolves three review
threads or appends a row to a tracking sheet. Uniform Opus burns tokens with no
quality return on the long tail of mechanical work, and the token-economics P0
makes that waste a first-order concern.

This spike defines *which* model runs at *which* role, *how* we measure the
quality-cost tradeoff so tiering is data-driven rather than asserted, and *what*
the first implementation bead (W0) must deliver.

## 1. Tiering matrix

| Role | Task type | Model | Rationale |
|---|---|---|---|
| Meta-Orchestrator | mesh supervision | **Opus** | Top of the failover hierarchy; reasoning errors cascade swarm-wide. Never tier down. |
| Orchestrator | dispatch / sequencing | **Opus** | WSJF sequencing, cap management, off-task kills — judgment-heavy. |
| Overseer | health / gate enforcement | **Opus** | Swap/FD gates, DoD violations, ghost-text detection. Cheap to run, expensive to get wrong. |
| Worker | docs-spike | **Sonnet** | Structured prose against a clear brief; Opus rarely changes the outcome. |
| Worker | code-impl | **Sonnet** (Opus on retry) | Most implementation is tractable on Sonnet; escalate complex beads. |
| Worker | code-review | **Sonnet** | Diff-scoped reasoning; pairs well with adversarial verification. |
| Worker | triage | **Haiku** | Classification + routing; high volume, low per-item stakes. |
| Worker | mechanical (thread-resolve, sheet-append, label) | **Haiku** | Deterministic, schema-bound; Opus is pure waste here. |

Default worker tier is **Sonnet**; Haiku is opt-in for mechanical/triage work,
Opus is opt-in for the hardest beads. Supervisors are pinned to Opus and are
**out of scope** for tiering-down — the mesh's stability budget dominates their
token cost.

## 2. Quality-cost metrics

"Task outcome quality" must be observable from the trail, not subjective. We
track per-worker:

- **PR merge success** — did the worker's PR auto-merge without human rescue?
- **No off-task kill** — did the orchestrator kill it for ghost-text / drift?
- **Ghost-text rate** — claims of work (e.g. "checked PR merged") not backed by
  a real tool call. The dominant Sonnet/Haiku failure mode to watch.
- **P0/P1 bead close rate** — did the dispatched bead actually close?
- **Rework rate** — follow-up beads / reopened threads attributable to the run.

Cost is output tokens per task (already poolable from the budget accounting).
The headline metric is **quality-per-token by (role, task_type, model)** — a
tier is justified only if it holds quality roughly constant while cutting cost,
or improves quality enough to justify added cost.

## 3. Worker tier selection

The orchestrator picks a worker's tier at dispatch from signals it already has:

1. **Task type** (primary) — the matrix above is the default lookup.
2. **Bead priority** — P0/P1 may float a Sonnet task up to Opus; P3 may floor it
   to Haiku.
3. **Estimated token budget** — large-context beads favor Opus's reasoning depth;
   tiny beads favor Haiku.
4. **Retry budget** — first attempt on the default tier; on failure
   (off-task kill, CI red, ghost-text) **escalate one tier** and re-dispatch.
   Escalation is cheaper than a human rescue and self-corrects mis-tiered beads.

## 4. Measurement infrastructure

The trail (`~/.agm/vroom/trail.jsonl`) already carries `kind`, `ts`, `note`.
Add structured fields on the worker dispatch/close events so tiers are
comparable without log-scraping:

| Field | Example | Purpose |
|---|---|---|
| `model` | `sonnet` | tier under test |
| `task_type` | `code-review` | matrix key |
| `bead` | `ce-th8k` | join key to bead priority |
| `outcome` | `merged` \| `killed` \| `reopened` | quality signal |
| `out_tokens` | `48213` | cost |
| `escalated_from` | `haiku` | retry-tier tracking |

A nightly rollup aggregates these into a quality-per-token table by
(role, task_type, model) — the artifact that drives future matrix revisions.

## 5. Rollout strategy

1. **Shadow mode** — keep dispatching on Opus, but log the tier the matrix
   *would* pick. Validates the selector and seeds baseline quality-per-token at
   zero production risk.
2. **Graduated rollout** — enable real tiering for the lowest-stakes class first
   (mechanical → Haiku), watch ghost-text and rework for one cycle, then admit
   docs-spike and code-review to Sonnet. Supervisors stay Opus throughout.
3. **Fallback rule** — if a tier's quality-per-token regresses against the Opus
   baseline beyond a threshold, auto-revert that (task_type, model) cell to the
   next tier up and flag it. The retry-escalation in §3 is the per-task safety
   net; the fallback rule is the per-cell one.

## 6. W0 requirements (first implementation bead)

W0 makes tiering *possible and measurable*, not yet *automatic*:

1. **`agm session new --model {opus|sonnet|haiku}`** — per-session model
   selection (today every session is opus-200k).
2. **Supervisor model config** — pin meta-o/orch/overseer to Opus explicitly so
   tiering changes can never touch them.
3. **Measurement schema** — the trail fields in §4 plus the nightly rollup, so
   shadow mode (§5.1) can start collecting baselines immediately.

Tier *selection logic* (§3) and *fallback automation* (§5.3) are follow-on
beads; W0's job is the `--model` flag, the supervisor pin, and the telemetry to
prove tiering pays before it ships.
