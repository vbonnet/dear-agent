# Spike: Intelligence-Per-Token Optimization (ce-0624)

**Status:** Investigation complete
**Date:** 2026-06-21
**Bead:** ce-0624
**Type:** Spike (timeboxed; research/analysis only — no model-tier changes made)
**Depends on:** ce-bkxa (`agm usage` monitor, merged via PR #643)

---

## Context

We now have a usage monitor (`agm usage`, ce-bkxa) that parses the JSONL
transcripts Claude Code writes under `~/.claude/projects/**` and estimates USD
cost by model and session type. This spike runs it against the current archive
to (1) baseline cost per session type, and (2) decide whether supervisors
(orchestrator, meta-orch, overseer) should be upgraded from Sonnet to Opus.

**Headline finding — the premise is already ~90% true.** Supervisors are *not*
mostly on Sonnet. Across the full archive, **90% of supervisor spend is already
Opus** ($19.5K of $21.8K). The Sonnet→Opus "upgrade" for supervisors is largely
a fait accompli. The open levers are the *inverse*: workers run 64% on Opus for
repetitive coding, and the daily burn rate ($834–1,279/day) is 16–25× the $50
alert threshold ce-bkxa ships with.

### Method & caveats

- Data source: `agm usage` plus a cross-tab replicating its exact heuristic.
- **Session-type heuristic** (from `usage.go:107`): cwd under
  `~/.agm/sandboxes/` ⇒ **worker**; everything else ⇒ **supervisor**. The JSONL
  carries no AGM role tag, so finer roles (orchestrator vs meta-orch vs
  overseer) are *not* separable from transcripts alone — they collapse into
  "supervisor." Attempts to classify by prompt-header (`# Worker:` /
  `# Orchestrator:`) recovered only 1/2204 files; the header is not persisted in
  a parseable position. This is the main fidelity gap.
- **Cost is estimated** from approximate public pricing; transcripts record
  tokens, not prices. Cache-aware costing applied (cache-create ×1.25,
  cache-read ×0.10 of input rate). Pricing used below.
- Archive scanned: **2,158 transcript files / 295 session dirs**, project
  inception → 2026-06-21. Data is **not sparse** — the baseline is solid.

| Model | $/Mtok in | $/Mtok out |
|---|---|---|
| Opus 4.x (4.6/4.7/4.8) | 15.00 | 75.00 |
| Sonnet 4.x (4.5/4.6) | 3.00 | 15.00 |
| Fable 5 | 5.00 | 25.00 *(est.)* |
| Haiku 4.5 | 0.25 | 1.25 |

---

## 1. Current baseline

### By model (full archive)

| Model | Messages | Cost | Share |
|---|---:|---:|---:|
| claude-opus-4-8 | 67,948 | $20,271.66 | 72.7% |
| claude-sonnet-4-6 | 94,265 | $3,944.38 | 14.2% |
| claude-opus-4-7 | 6,143 | $1,927.10 | 6.9% |
| claude-opus-4-6 | 5,232 | $1,238.57 | 4.4% |
| claude-fable-5 | 4,064 | $448.17 | 1.6% |
| claude-sonnet-4-5 | 1,257 | $25.66 | 0.1% |
| claude-haiku-4-5 | 6,423 | $13.66 | 0.0% |
| **Total** | **185,332** | **$27,869.20** | **100%** |

**Opus (all versions) = $23,437 (84%)** of spend on 79,323 messages.
**Sonnet (all) = $3,970 (14%)** on 95,522 messages — Sonnet handles *more*
message volume than Opus at one-sixth the cost.

### By session type × model (the decisive cross-tab)

| Session | Model | Cost | Messages | $/msg |
|---|---|---:|---:|---:|
| supervisor | opus | $19,531.87 | 62,329 | 0.313 |
| worker | opus | $3,910.08 | 17,014 | 0.230 |
| worker | sonnet | $2,203.27 | 53,072 | 0.042 |
| supervisor | sonnet | $1,767.35 | 42,464 | 0.042 |
| supervisor | fable | $448.17 | 4,064 | 0.110 |
| worker/sup | haiku | $13.66 | 6,423 | 0.002 |

| Session type | Total cost | Share of spend | % on Opus | % on Sonnet |
|---|---:|---:|---:|---:|
| **supervisor** | **$21,760** | **78%** | **90%** | 8% |
| **worker** | **$6,114** | **22%** | 64% | 36% |

**Supervisors already run 90% Opus and are 78% of all spend.** Workers — the
"repetitive coding" tier — run 64% Opus.

### Burn rate (live, via `agm usage`)

| Window | Total | Daily burn rate | Top model |
|---|---:|---:|---|
| Last 24h | $1,279.18 | **$1,279/day** | opus-4-8 (77%) |
| Last 7d | $5,840.27 | **$834/day** | opus-4-8 (73%) |

Per-transcript cost: mean $12.91, median $5.29, **p90 $25.89, max $852**. The
top 10 transcripts alone are 15% of all spend — a heavy right tail driven by
long-running supervisor sessions.

> ⚠️ **The $50/day burn-rate alert threshold is 16–25× below actual burn.** At
> current fleet scale it never fires meaningfully (`alert:false` even at
> $1,279/day because no realistic threshold is set). This needs recalibration
> independent of any model-tier decision.

---

## 2. Model tier matrix — Sonnet vs Opus per session type

Observed **effective** cost ratios (cache- and output-mix-adjusted), not catalog:

| Tier | Sonnet $/msg | Opus $/msg | Effective ratio | Catalog ratio |
|---|---:|---:|---:|---:|
| supervisor | 0.042 | 0.313 | **~7.5×** | 5× |
| worker | 0.042 | 0.230 | **~5.5×** | 5× |

Effective ratio exceeds the 5× catalog ratio because Opus supervisor sessions
are output- and reasoning-heavy (planning, recovery, review), where the 75/15
output-price gap dominates.

| Session type | Work profile | Quality sensitivity | Tier verdict |
|---|---|---|---|
| **Orchestrator / meta-orch** | Prioritization, decomposition, recovery from stuck workers — high-leverage, low-volume, errors cascade fleet-wide | **High** — a bad dispatch wastes N worker-hours | **Opus justified** (already there) |
| **Overseer** | Monitoring, anomaly/stall detection, escalation | Medium-high — misses are expensive but rare | **Opus defensible**; Sonnet viable for pure polling |
| **Worker (design/spec)** | Architecture, API design within a task | Medium-high | Opus or Fable |
| **Worker (codegen/mechanical)** | Repetitive edits, test scaffolding, lint fixes | **Low** — verifiable by CI, cheap to retry | **Sonnet/Haiku is the win** |

**Cost-to-finish-the-upgrade.** Moving the residual 8% supervisor-Sonnet
($1,767) fully to Opus costs roughly +$7–9K at equal token volume (×5–7.5).
That is the *real* price of "supervisors on Opus" — most of it is already paid;
the rest is non-trivial but bounded.

---

## 3. Recommendation

**Do not frame this as "upgrade supervisors to Opus" — that is ~90% done.**
Reframe the goal as *intelligence-per-token*: put Opus where leverage is high and
pull it back where work is mechanical and CI-verifiable.

1. **Supervisors → keep on Opus; formalize it.** Orchestrator/meta-orch/overseer
   already run 90% Opus and this is correct: they are 78% of spend but it buys
   fleet-wide leverage (one bad orchestration decision wastes many worker-hours).
   Make it an explicit policy/default rather than emergent, and convert the
   residual 8% Sonnet supervisor traffic deliberately (cost: +$7–9K/equiv-volume,
   accept it).

2. **The bigger lever is workers, not supervisors.** Workers run **64% Opus**
   for largely repetitive, CI-verifiable coding. Routing mechanical worker
   sub-tasks (lint/test/codegen) to Sonnet or Haiku is the highest-ROI move:
   even halving worker-Opus ($3,910) toward Sonnet saves ~$1,600 at current
   volume with low quality risk, since CI catches regressions cheaply.

3. **Fix the burn-rate alert before anything else.** Actual burn is
   $834–1,279/day; the shipped $50 threshold is noise. Recalibrate to something
   actionable (e.g. alert on >50% over trailing-7d mean, currently ~$1,250/day),
   so quota headroom is actually visible.

4. **Quota headroom for Fable re-release.** Fable 5 is currently 1.6% of spend
   ($448). On re-release, supervisors/designers will pull toward it (5/25
   pricing — between Sonnet and Opus). With supervisors already pinned to the
   top tier, a Fable spike on top of $834–1,279/day Opus burn has **little
   headroom**. This argues *against* further upgrading and *for* the worker
   downgrade in (2) to bank headroom now.

**Net:** the supervisor-upgrade question is essentially closed (yes, and it's
done). The actionable optimization is the opposite direction — disciplined
worker-tier downgrades plus a working burn alert.

---

## 4. Next actions

If leadership accepts the reframing:

1. **Recalibrate the burn alert (ce-bkxa follow-up).** Set a dynamic threshold
   (trailing-7d mean × 1.5) instead of the static $50. *Metric:* alert fires
   only on genuine anomalies, not every day. **(Do first — cheap, unblocks
   visibility.)**
2. **Codify supervisor=Opus as policy.** Document the default and migrate the 8%
   residual Sonnet supervisor traffic. *Metric:* supervisor-Sonnet share → ~0%;
   watch supervisor $/msg stays ≈0.31.
3. **Pilot worker-tier routing.** Route mechanical worker sub-tasks (lint, test
   scaffolding, codegen) to Sonnet/Haiku on a labeled task class first. *Metric
   that proves value:* worker-Opus share drops from 64% with **no rise in CI
   failure rate or rework-loop count** — that's the intelligence-per-token win.
4. **Add per-role tagging to transcripts.** The biggest analysis gap is that
   orchestrator/meta-orch/overseer collapse into "supervisor." Emit an AGM role
   tag into the session so `agm usage` can break supervisor spend down by actual
   role. *Metric:* future spikes can target the most expensive *role*, not just
   the coarse worker/supervisor split.

---

## Appendix — reproduction

```bash
# Canonical baseline (ce-bkxa monitor):
cd ~/src/dear-agent/agm && go run ./cmd/agm usage --since 168h --output json

# Session-type × model cross-tab uses the same cwd heuristic as usage.go:107
#   (~/.agm/sandboxes/ => worker, else supervisor), cache-aware costing.
```

All figures: full `~/.claude/projects/**` archive as of 2026-06-21. Costs are
estimates from public pricing; treat as directional, not invoice-accurate.
