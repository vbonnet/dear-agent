# Spike: Intelligence-Per-Token Optimization (ce-0624)

**Status:** Spike / research only — no model-tier changes made here.
**Depends on:** ce-bkxa (`cmd/cc-usage-monitor`, merged via PR #643).
**Date:** 2026-06-22

## TL;DR

- The fleet **already runs predominantly on Opus** — `claude-opus-4-8` is **71% of
  measured spend** (~$21k of ~$29.7k all-time transcript cost). The framing
  "upgrade supervisors from Sonnet to Opus" partly mis-describes the current
  state: most spend is *already* Opus; Sonnet is the high-volume / low-cost tier.
- A Sonnet→Opus flip costs **~7.1× per message** (measured: $0.295/msg Opus-4-8 vs
  $0.042/msg Sonnet-4-6).
- **Recommendation: targeted yes.** Put *supervisors* (orchestrator, meta-orch,
  overseer) on Opus and keep *workers* on Sonnet by default. Supervisors are
  low-volume + high-leverage; that's where Opus's marginal intelligence pays for
  itself. Do **not** blanket-upgrade workers.
- **Blocker for measuring value:** the usage monitor currently classifies **0
  sessions as `supervisor`** and dumps 76.5% of spend into an `unknown` bucket.
  We cannot prove supervisor-upgrade ROI until session-type tagging is fixed.

## 1. Current baseline

Data source: `cmd/cc-usage-monitor -json` over `~/.claude/projects/**/*.jsonl`
(2,259 transcript files), cross-checked with a full cache-aware re-pricing across
all model tiers (the shipped tool only prices Opus IDs — see §5 caveats).

### Burn rate (live)

| Metric | Value |
|---|---|
| Projected 24h cost (1h window) | **$1,883** |
| Projected 24h cost (24h window) | **$2,720** |
| Alert threshold (ce-bkxa default) | $50/day |
| Status | 🔴 **BURN ALERT firing** — ~38–54× over threshold |

The $50/day threshold is a smoke alarm, not a budget; the fleet runs far above it.
It still earns its keep as a *delta* signal (catching runaway loops), but the
static number is stale for a multi-agent mesh.

### Cost by model (all-time transcripts, full cache-aware pricing)

| Model | Cost | Share | Messages | $/msg |
|---|---:|---:|---:|---:|
| claude-opus-4-8 | $21,049 | 71.0% | 71,269 | $0.295 |
| claude-sonnet-4-6 | $4,023 | 13.6% | 96,159 | $0.042 |
| claude-opus-4-7 | $1,927 | 6.5% | 6,143 | — |
| claude-fable-5 | $1,345 | 4.5% | 4,064 | $0.331* |
| claude-opus-4-6 | $1,239 | 4.2% | 5,232 | — |
| claude-haiku-4-5 | $55 | 0.2% | 6,477 | $0.009 |
| claude-sonnet-4-5 | $26 | 0.1% | 1,257 | — |
| **Total** | **$29,663** | 100% | 190,601 | |

\* Fable priced at Opus tier as a placeholder (no public Fable-5 rate card yet).

**Read:** Sonnet handles the *most* messages (96k, the workhorse tier) for 14% of
cost. Opus is 37% of messages but 71% of cost. The cost center is Opus volume, not
Sonnet.

### Cost by session type

| Type | Cost | Share | Messages | Dominant models |
|---|---:|---:|---:|---|
| unknown | $22,688 | 76.5% | 114,807 | opus-4-8 $16.4k, opus-4-7 $1.9k, sonnet $1.7k |
| worker | $6,975 | 23.5% | 76,773 | opus-4-8 $4.6k, sonnet $2.3k |
| **supervisor** | **$0** | **0%** | **0** | — (none classified) |

The headline finding for this spike: **we cannot currently see supervisor spend at
all.** `ClassifySessionType` keys off path markers (`orchestrator`, `overseer`,
`supervisor`, `meta-o`, `worker`, `sandbox`). Live supervisor sessions don't carry
those markers in their `cwd`, so every one of them lands in `unknown`. Any claim
about "supervisor cost" today is an estimate, not a measurement.

## 2. Model tier matrix (Sonnet vs Opus)

Cost factors are measured ($/msg ratio); quality deltas are qualitative judgement
from how each session type fails.

| Session type | Volume | Work character | Sonnet→Opus cost | Quality delta on Opus | Verdict |
|---|---|---|---:|---|---|
| **Orchestrator** | Low | Prioritization, dependency ordering, recovery from stuck workers | ~7.1× | **High** — fewer bad dispatch decisions; one avoided wrong-task fan-out saves many worker-hours | **Upgrade** |
| **Meta-orchestrator** | Very low | Cross-orchestrator arbitration, design | ~7.1× | **High** — highest leverage, lowest volume → cheapest upgrade in absolute $ | **Upgrade** |
| **Overseer** | Low | Drift/safety review, catching bad merges | ~7.1× | **Med-High** — catching one bad merge > the Opus premium | **Upgrade** |
| **Worker (coding)** | High | Repetitive edits against a clear spec | ~7.1× | **Low-Med** — Sonnet already strong on scoped coding; Opus rarely changes the diff | **Keep Sonnet** |
| **Mechanical/lint** | High | Formatting, deterministic transforms | ~7.1× | **~None** | **Keep Sonnet / Haiku** |

### Upgrade cost model (if supervisors currently ran Sonnet)

Assuming supervisors are 10–20% of message volume:

| Supervisor share | Msgs | Cost on Sonnet | Cost on Opus | Delta |
|---|---:|---:|---:|---:|
| 10% | 19,060 | $797 | $5,629 | +$4,832 |
| 15% | 28,590 | $1,196 | $8,444 | +$7,248 |
| 20% | 38,120 | $1,595 | $11,259 | +$9,664 |

These are upper bounds — supervisors are lower-volume than workers (more thinking
per message, fewer messages), so the true share is likely the low end. The
absolute premium for upgrading *only* supervisors is small relative to the $21k the
fleet already spends on Opus workers.

## 3. Recommendation

**Targeted upgrade: supervisors on Opus, workers on Sonnet.** Rationale:

1. **Leverage asymmetry.** Supervisors make few, high-consequence decisions
   (what to build, what to abandon, what to recover). A single bad
   prioritization or a missed bad merge costs more than the entire Opus premium
   for that session. Workers execute scoped specs where Sonnet's quality is
   already at the ceiling for the task.
2. **Cheap in absolute terms.** Supervisors are low-volume; even the 20% upper
   bound is <$10k against a $21k existing Opus bill. The marginal spend is small
   and bounded.
3. **Don't blanket-upgrade workers.** Workers are 96k+ Sonnet messages. Flipping
   them to Opus is the 7.1× multiplier applied to the highest-volume tier — that's
   the expensive, low-return move. Avoid it.
4. **Fable re-release headroom.** Fable-5 already appears (4,064 msgs, ~$1.3k at
   Opus-tier pricing). A Fable re-release at scale will spike costs on a tier we
   don't yet have a confirmed rate card for. Keeping workers on Sonnet preserves
   quota headroom for that spike. Reserve the premium tiers for supervisors.
5. **We now have visibility (ce-bkxa) — use it as a gate, not a green light.**
   Quota visibility exists, but until supervisor sessions are *classified* the
   monitor can't attribute the new spend. Fix tagging first (see §4).

**Caveat to the framing:** the fleet is already Opus-heavy. The real optimization
isn't "spend more on Opus for supervisors" — it's "stop spending Opus on workers
that Sonnet handles," then redirect that headroom to supervisors. The
intelligence-per-token win is a *reallocation*, not a net increase.

## 4. Next actions

1. **Fix session-type classification (prerequisite — blocks everything).**
   `ClassifySessionType` must tag supervisor sessions reliably. Options: stamp the
   role into the session `cwd`/name at spawn time (agm/vroom), or emit an explicit
   `session_role` field into the transcript. Today 76.5% of spend is `unknown` and
   0% is `supervisor` — no ROI can be proven without this. *File as a follow-up
   bead.*
2. **Pilot order:** meta-orchestrator → orchestrator → overseer. Start with
   meta-orch (lowest volume, highest leverage, cheapest to flip).
3. **Proof metric:** *task-success rate per supervisor-dollar* and *worker-rework
   rate* (fraction of dispatched tasks that bounce back / get re-dispatched).
   Hypothesis: Opus supervisors reduce worker rework enough that total fleet cost
   per *completed* task drops even though supervisor cost rises. Measure rework
   rate Sonnet-baseline for 1 week, flip meta-orch+orch to Opus, compare.
4. **Recalibrate the burn alert.** $50/day is ~40× below reality. Replace with a
   per-window *delta* alert (e.g. >2× trailing-7d baseline) plus a real daily
   budget ceiling. Keep the absolute alarm only for single-session runaway loops.
5. **Fix the tool's pricing table.** `cc-usage-monitor` prices only Opus IDs and
   reports $0 for Sonnet/Fable/Haiku, undercounting true spend by ~$4k+. Add the
   full rate card (and a placeholder for Fable-5) so `by_model` cost is accurate.

## 5. Caveats & method

- **Pricing model:** per-Mtok USD — Opus 15/75, Sonnet 3/15, Haiku 1/5; cache
  write ≈1.25× input, cache read ≈0.1× input. Fable-5 priced at Opus tier as a
  placeholder (no public rate card). Update when Fable pricing is published.
- **Window semantics:** `cc-usage-monitor` aggregates the full report cumulatively
  and uses `-window` only for the burn-rate *projection*; 7d and 24d report totals
  are near-identical because the corpus is the same. "All-time" = whatever
  transcripts currently live under `~/.claude/projects`.
- **Shipped-tool gap:** the tool's `by_model` cost zeroes non-Opus models; the
  by-model/by-type cost tables above use the corrected full-tier re-pricing. The
  burn-rate and token/message counts are taken directly from the tool.
- **Classification gap:** `supervisor`=0 is a measurement artifact, not evidence
  that supervisors are free. See action #1.
