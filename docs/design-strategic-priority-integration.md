# Design Spike: Strategic Priority Integration

**Bead:** ce-9m96 · **Status:** Investigation only (no implementation) · **Date:** 2026-06-21
**Companions:** [[ce-48vk]] P1 reprioritization · [[ce-h59v]] grooming cadence · [[ce-46wz]] tier implementation

---

## Problem

The mesh carries **165 open P1 beads** — if everything is P1, nothing is.
[[ce-h59v]] decided *who* grooms and *when*; [[ce-46wz]] decided *how often* each
tier fires. Neither pins down *how a bead's strategic alignment gates whether it
gets worked at all.* This spike documents the connective tissue: a **theme
taxonomy**, a **label convention**, a **grooming gate** the orchestrator checks
before dispatch, and the **two-pass MoSCoW→ICE triage** that produces a defensible
priority. No schema change — label discipline plus existing `bd` metadata.

Storage convention is inherited (**metadata = query, trail = audit**, per
[[ce-46wz]]): theme labels and ICE scores live in queryable `bd` state; every
triage action lands a `kind`-tagged line in `~/.agm/vroom/trail.jsonl`.

---

## 1. Strategic theme taxonomy

Seven themes, ranked. The number **is** the priority weight — theme 1 outranks
theme 7. The stack is the single source of truth from [[ce-48vk]]:

| # | Theme | Shorthand |
|---|-------|-----------|
| 1 | OTel activation | `strategic_theme:1` |
| 2 | Brakes / circuit breaker | `strategic_theme:2` |
| 3 | Memory-to-repo migration + grooming | `strategic_theme:3` |
| 4 | Retro aggregation + quality metrics | `strategic_theme:4` |
| 5 | Spec-driven beads | `strategic_theme:5` |
| 6 | OAuth hardening | `strategic_theme:6` |
| 7 | 24/7 continuity | `strategic_theme:7` |

**Priorities 1–3 are the active focus.** Themes 4–7 are real but yield to 1–3 in
any contention. Re-ranking the stack is a **quarterly** act ([[ce-46wz]] §1),
never a per-session one.

## 2. Label convention

One label per bead, format `strategic_theme:N` (N ∈ 1–7). No schema change — it is
an ordinary `bd` label:

```
bd --db ~/beads/context-engine/.beads label add ce-XXXX strategic_theme:1
bd --db ~/beads/context-engine/.beads list --label strategic_theme:1 --status open --json
```

**Standing rule:** every new bead gets a theme label at creation, alongside the
P2 default from [[ce-h59v]]. A bead with no theme label is an **orphan** and is
caught by the biweekly coverage sweep ([[ce-46wz]] §2). A bead that fits no theme
is a signal — either the work is off-strategy (close it) or a theme is missing
(flag for the quarterly re-rank).

## 3. Grooming gate protocol

Before the orchestrator dispatches a bead to a worker, it runs a three-question
gate. **First failure stops dispatch** — the bead is parked, not silently dropped.

1. **P0?** — If P0, dispatch regardless of theme (blockers override strategy). Skip
   to dispatch.
2. **Theme aligned?** — Does the bead carry `strategic_theme:1`, `:2`, or `:3`?
   If not, and it is not P0, **defer** and flag for triage (it may be real work in
   a lower theme — the triage pass decides, not the gate).
3. **ICE above threshold?** — Is the stored `ice.score` ≥ 27 (the Should floor,
   §5)? Below 27 → defer to triage; the bead is a Could/Won't and does not warrant
   a worker mid-cycle.

Pass all three → dispatch. Each deferral emits
`kind: orchestrator.dispatch.gated` with the failing question, so deferrals are
auditable rather than invisible.

## 4. MoSCoW triage procedure

Pass one is a **fast categorical sort** — cheaper than scoring, it bins the
backlog before the expensive ICE pass touches only the top buckets.

| Bucket | Means | Lands at |
|--------|-------|----------|
| **Must** | Blocker or theme-1–3 commitment this cycle | P0 (consensus gate) |
| **Should** | Theme-aligned, high value, not blocking | P1 |
| **Could** | Real but deferrable; lower theme | P2 |
| **Won't (now)** | Off-strategy or superseded | P3 / close |

Procedure: pull all P0+P1 beads, drop each into one bucket by judgment (theme rank
+ "would we drop this in a crunch?"). **Won't** beads are closed with a link to
the superseding bead or archived — never left to rot as zombie P1s. Only **Must**
and **Should** survive into pass two. Target after both passes ([[ce-48vk]]):
**≤16 P0 (10%), ≤32 P1 (20%)**, ≤30 true P1s.

## 5. ICE scoring guide

Pass two scores the Must/Should survivors. **ICE = Impact × Confidence × Ease**,
each dimension **1–5**, product **1–125** (convention inherited from [[ce-46wz]]
§3 — kept identical so scores are comparable across docs).

| Dim | 1 | 3 | 5 |
|-----|---|---|---|
| **Impact** | cosmetic / one agent | one subsystem | whole mesh / unblocks a theme |
| **Confidence** | speculative | plausible, some unknowns | proven, scoped |
| **Ease** | weeks / cross-cutting | days, contained | hours, mechanical |

Score → bucket → priority:

| ICE | Bucket | Priority |
|-----|--------|----------|
| 64–125 | Must | P0 (consensus gate) |
| 27–63 | Should | P1 |
| 8–26 | Could | P2 |
| 1–7 | Won't (now) | P3 / defer |

Stored as metadata, not a native field:

```
bd update ce-XXXX --set-metadata ice.impact=4 --set-metadata ice.confidence=3 \
  --set-metadata ice.ease=5 --set-metadata ice.score=60
```

**Tie-break:** equal ICE → lower theme number wins. **P0 promotions defer to the
orch+overseer consensus gate** ([[ce-h59v]] §1) — ICE recommends, consensus
ratifies.

## 6. Orchestrator integration

Two touch-points, both on the existing tick — no new loop:

```
on dispatch(bead):              # §3 gate — every dispatch, cheap
  if bead.priority == P0: dispatch(); return
  if bead.theme not in {1,2,3}: defer(bead, reason=theme); return
  if bead.metadata.ice.score < 27: defer(bead, reason=ice); return
  dispatch()

on tick:                        # piggybacks the ce-h59v grooming clock
  if grooming_due(tick) and gate == OPEN:
    moscow_sort(P0+P1)          # §4 pass one
    ice_rescore(must, should)  # §5 pass two — reuses ce-46wz weekly rescore
    emit_trail(grooming_complete)
```

The gate is **per-dispatch and synchronous**; the rescore is the **weekly tier**
from [[ce-46wz]] §3 — this spike adds no cadence, it reuses that one and feeds the
gate a fresh `ice.score`.

## 7. W0 requirements

Thinnest runnable slice:

1. This process doc (the convention is the deliverable).
2. **One scored pass through the current P1 backlog** — label all 165 with a
   theme, run MoSCoW, ICE-score the Must/Should survivors, write `ice.score`
   metadata, land the post-groom counts in the trail.

That single pass both proves the procedure and discharges [[ce-48vk]]. No new `bd`
field is required for W0 — labels + metadata + the existing weekly rescore are
sufficient. Native `ice.score` sort/filter is a fast-follow only if the shell glue
becomes painful at volume ([[ce-46wz]] §6).
