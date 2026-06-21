# Design Spike: WSJF Sequencing for Bead Backlog Prioritization

**Bead:** ce-htwo · **Status:** Investigation only (no implementation) · **Date:** 2026-06-21
**Companion:** [[ce-h59v]] grooming cadence & ownership

## Problem

Prioritization is ordinal P0–P4 with no systematic sequencing *within* a tier.
With 30 open P0 and 165 open P1 beads, "what next?" inside a band is decided ad
hoc. WSJF (Weighted Shortest Job First) gives a reproducible intra-tier order:

> **WSJF = Cost of Delay / Job Duration**, where
> **CoD = Business Value + Time Criticality + Risk Reduction/Opportunity Enablement**

The intent is *sequencing clarity within a tier*, not replacing P0–P4. P-tier
stays the coarse SLA; WSJF orders beads that share a tier.

## 1. Scoring Model

- **Scale:** modified Fibonacci `1, 2, 3, 5, 8, 13` for each of BV, TC, RR, and
  Job Duration (JD). Non-linearity forces a decision at the high end instead of
  defaulting everything to "5".
- **Who scores:** the **meta-orchestrator** during grooming (it owns roadmap
  value). Workers may *propose* a JD revision when a spike reveals true effort —
  JD is the field workers know best after touching the code.
- **Relative, not absolute:** scores are comparative within a grooming batch.
  Calibrate by anchoring one obvious "13 CoD / 1 JD" and one "low" bead.
- **Re-score cadence:** only TC drifts with time, so a **full re-score every
  grooming cycle (~480 ticks / daily)** plus an event-driven bump when a bead
  becomes a hard blocker. Avoid per-bead continuous re-scoring — it is the main
  overhead trap.

## 2. Storage

`bd` already exposes the needed fields — no new tracking file:

- **JD → `--estimate`** (native minutes field). Map Fibonacci to minutes
  (1≈15m, 13≈1d) so JD is reusable by scheduling.
- **BV/TC/RR + computed WSJF → `--set-metadata`** (native JSON KV):
  `--set-metadata wsjf.bv=8 --set-metadata wsjf.tc=13 --set-metadata
  wsjf.rr=5 --set-metadata wsjf.score=8.7 --set-metadata wsjf.scored_at=<tick>`.
- **Why metadata, not trail:** metadata is queryable/sortable for "next bead";
  the trail is append-only history. The scoring *event* still lands in the trail
  automatically, giving an audit of re-scores for free.

Sequencing query: `bd list` sorted by `wsjf.score` desc, within P-tier.

## 3. Grooming Integration

WSJF folds into the existing [[ce-h59v]] checklist as a new step **after** P-tier
triage and dedup (don't score zombies/dupes):

1. P0 audit → 2. P1 triage → 3. dedup → … → **N. WSJF score** the surviving
   open P0/P1 set → emit sorted "ready queue".

- **Meta-orchestrator computes** BV/TC/RR/score; this stays a supervisor
  responsibility, consistent with ce-h59v ("grooming NOT delegated to workers").
- **Workers consume** the sorted queue and may file a JD-correction note.
- WSJF is **advisory within a tier** — it never promotes a P2 above a P0. A P0
  always outranks any P1 regardless of score.

## 4. Pilot — Top Open P0/P1 Beads

Scored by inspection of current `bd list` (Fibonacci; WSJF = (BV+TC+RR)/JD):

| Bead | What | BV | TC | RR | CoD | JD | **WSJF** |
|---|---|--:|--:|--:|--:|--:|--:|
| ce-7hhv | AGM control plane down — blocks *all* agm ops + relay | 13 | 13 | 8 | 34 | 3 | **11.3** |
| ce-5i6o | zero-bypass merge ruleset on main | 5 | 3 | 13 | 21 | 3 | **7.0** |
| ce-710r | gopls FD leak — blocks all `go build` | 13 | 8 | 8 | 29 | 5 | **5.8** |
| ce-95yt | repoint burndown skill off ~/src | 3 | 3 | 5 | 11 | 2 | **5.5** |
| ce-kswe | session-end orphan-process reaper | 5 | 5 | 5 | 15 | 3 | **5.0** |
| ce-6as.10 | Gmail OAuth dead ~3mo (triage broken) | 3 | 2 | 2 | 7 | 2 | **3.5** |
| ce-mb9g | merge→binary deployment-verify gate | 5 | 3 | 8 | 16 | 5 | **3.2** |
| ce-84l2 | supervisors can't run autonomously | 8 | 8 | 5 | 21 | 8 | **2.6** |

**Does it match intuition?** Mostly. ce-7hhv (total outage, cheap restart) at the
top is obviously right. Two instructive frictions:

- **ce-5i6o jumps above ce-710r** despite ce-710r being an *active* build-blocker.
  WSJF rewards ce-5i6o's high Risk-Reduction + small job. This is a *feature* —
  it surfaces cheap risk-burndown — but a human would likely still fix the active
  blocker first. **Mitigation:** an active-blocker flag overrides WSJF (TC=13
  forcing function), or treat ties within ~1.5× as human's-choice.
- **ce-84l2 sinks to last** purely because JD=8. This is WSJF's known bias against
  large-but-important work; it must be split into smaller beads (which it should
  be anyway) rather than left to rot at the bottom.

Net: WSJF produced a *defensible* order and exposed exactly the two judgment
calls a human should make — which is the goal.

## 5. Overhead vs Value

- **Cost:** ~4 integer estimates per bead. For ~195 beads, a *full* re-score is
  hours; therefore score **only the open P0/P1 ready set (~30–40 beads)**, ~10–15
  min/cycle once calibrated. Re-scoring drift (TC only) is cheap.
- **Value:** removes per-pull "what next?" deliberation across every worker
  dispatch, and makes sequencing *auditable* (metadata + trail). For an
  autonomous mesh pulling continuously, a stable machine-readable order is worth
  more than for a human team.
- **Verdict:** positive ROI **only if scoped to the ready set**. Scoring the full
  backlog is net-negative overhead — that is the failure mode to avoid.

## 6. W0 Requirements (implementation bead)

An impl bead must deliver:

1. **Metadata schema:** `wsjf.{bv,tc,rr,score,scored_at}` convention + JD via
   `--estimate`; documented in grooming runbook.
2. **`bd` query/recipe:** `bd ready` (or wrapper) sorted by `wsjf.score` desc,
   bucketed by P-tier; active-blocker override.
3. **Grooming-step automation:** a meta-orchestrator routine that scores the open
   P0/P1 set, writes metadata, and emits the sorted queue once per cycle.
4. **Calibration anchors** doc (one high-WSJF, one low example) for score
   consistency across cycles.
5. **Guardrails:** WSJF never crosses P-tier boundaries; ties within 1.5× flagged
   for human review; large-JD beads (JD≥8) flagged "split me".
6. **Acceptance:** the pilot ordering above reproduces from stored metadata via
   the query, no separate tracking file.
