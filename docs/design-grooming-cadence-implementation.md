# Design Spike: 3-Tier Grooming Cadence Implementation

**Bead:** ce-46wz · **Status:** Investigation only (no implementation) · **Date:** 2026-06-21

## Problem

[[ce-h59v]] decided *what* grooming is and *who* owns it: the meta-orchestrator
runs a daily-or-triggered session against a fixed checklist. It did not specify
*how* the work decomposes over time. Running the full checklist every session is
wasteful — re-ranking strategic themes daily is noise, while stale ICE estimates
rot between rare full passes. This spike splits grooming into **three nested
tiers** at different periods, plus a **90-day auto-decay** sweep, and pins each
to concrete `bd` commands and a supervisor integration point.

Storage convention is inherited from the lifecycle work ([[ce-blct]], ce-t8kn):
**metadata = query, trail = audit.** ICE scores and decay flags live in queryable
`bd` metadata; every session and decay action lands a `kind`-tagged line in
`~/.agm/vroom/trail.jsonl`.

## Tier model

| Tier | Period | Scope | Cost |
|------|--------|-------|------|
| Quarterly | ~43,200 ticks (~90d) | Full stack re-rank — strategic theme priorities | High (1 session) |
| Biweekly | ~6,720 ticks (~14d) | Theme→bead coverage; orphan detection | Medium |
| Weekly | ~3,360 ticks (~7d) | ICE rescore within MoSCoW buckets | Low (most frequent) |
| Auto-decay | every session | Flag beads unworked > 90d for triage | Trivial (one query) |

Tiers nest: a quarterly tick is also a biweekly and weekly tick, so the supervisor
runs the *superset* checklist that fires. The 480-tick/daily grooming clock from
[[ce-h59v]] is the carrier; tier work is gated by `tick % period` checks inside it.

## 1. Quarterly: full stack re-rank

Re-rank strategic themes, then cascade priority to their beads.

```
bd --db ~/beads/context-engine/.beads list --priority-max P1 --json --limit 0
bd --db ~/beads/context-engine/.beads list --label theme --json
```

1. Pull all P0+P1 beads and the theme set.
2. For each theme, score on ICE (see §3) at the *theme* level and rank themes.
3. Cascade: a bead cannot outrank its theme. Bulk-apply the ceiling:
   `bd update <id1> <id2> … --priority P1` (multi-ID update is native).
4. Record before/after theme order in the trail
   (`kind: supervisor.meta.grooming_quarterly`).

This is the only tier permitted to move P0s, and only through the orch+overseer
consensus gate from [[ce-h59v]] §1.

## 2. Biweekly: theme→bead coverage

Detect two drift modes: **orphaned beads** (no theme label) and **uncovered
themes** (a theme with no active beads).

```
# orphans: open beads carrying no theme label
bd --db ~/beads/context-engine/.beads list --no-labels --status open --json
bd --db ~/beads/context-engine/.beads list --label-pattern 'theme-*' --json

# coverage: for each theme T, does any open bead carry it?
bd --db ~/beads/context-engine/.beads count --label theme-<T> --status open
```

Orphans → assign the best-fit theme label or, if none fits, flag for theme
creation. Themes with zero open beads → either retire the theme or seed a bead.
`bd find-duplicates` runs here too, folding semantic dupes into a canonical bead
(close dupes with a link, per [[ce-h59v]] §3). Trail:
`kind: supervisor.meta.grooming_biweekly` with orphan/uncovered counts.

## 3. Weekly: ICE rescore

ICE = **Impact × Confidence × Ease**, each 1–5, product 1–125. Rescore catches
estimates that drifted as context changed. Score → MoSCoW/priority mapping:

| ICE score | Bucket | Priority |
|-----------|--------|----------|
| 64–125 | Must | P0 (consensus gate) |
| 27–63 | Should | P1 |
| 8–26 | Could | P2 |
| 1–7 | Won't (now) | P3 / defer |

Scores are stored as metadata, not a native field:

```
bd update ce-XXXX --set-metadata ice.impact=4 --set-metadata ice.confidence=3 \
  --set-metadata ice.ease=5 --set-metadata ice.score=60
bd list --has-metadata-key ice.score --json   # rescore candidates
```

The supervisor recomputes the product, compares to the current priority, and
bulk-corrects mismatches. P0 promotions defer to the consensus gate. Trail:
`kind: supervisor.meta.grooming_weekly`.

## 4. 90-day auto-decay

Unworked beads are **flagged for triage, never auto-closed** ([[ce-h59v]] intent).
The `bd` CLI already supports age queries — no new tooling needed:

```
bd --db ~/beads/context-engine/.beads stale --days 90 --status open --json
# or, equivalently, an explicit cutoff:
bd --db ~/beads/context-engine/.beads list --updated-before 2026-03-22 \
  --status open --json
```

Each result gets a decay flag and lands in the next human/supervisor triage view:

```
bd update <id> --add-label needs-triage --set-metadata decay.flagged_at=<tick>
```

Decay runs **every** session (it is one cheap query) so flags appear within a day
of crossing 90d, rather than waiting for a tier boundary. Each flag emits
`kind: supervisor.meta.bead_decayed` to the trail. Re-flagging is idempotent via
`decay.flagged_at` presence.

## 5. Supervisor integration

Grooming hangs off the existing meta-orchestrator tick — no new loop:

```
on tick:
  if not grooming_due(tick): return            # 480-tick clock OR P1>150/P0>25 trigger (ce-h59v)
  if swap_gate != OPEN: defer; return          # ce-h59v §4 timing gate — clock holds, doesn't skip
  run_decay_sweep()                            # §4, every session
  if tick % WEEKLY   == 0: run_weekly()        # §3
  if tick % BIWEEKLY == 0: run_biweekly()      # §2
  if tick % QUARTERLY== 0: run_quarterly()     # §1  (orch+overseer consensus for P0)
  emit_trail(grooming_complete, counts_before_after)
  handoff_advisory(orch, overseer)             # propagate post-groom state
```

Tier checks are modulo the same tick counter, so the nesting in the tier model
falls out for free. The handoff advisory at the end is how orch/overseer learn of
P0 changes without polling — same channel [[ce-h59v]] §5 names. Engram-research
picks up the trail stream for cross-cycle learning.

## 6. Tooling gap

Auditing the live `bd` CLI against this design, most assumed gaps are **already
closed**:

| Need | Status | Mechanism |
|------|--------|-----------|
| Bulk priority/label update | ✅ exists | `bd update <id1> <id2> …` accepts multiple IDs |
| Age / staleness filter | ✅ exists | `bd stale --days N`, `bd list --updated-before` |
| Metadata read/write | ✅ exists | `--set-metadata`, `--has-metadata-key`, `--metadata-field` |
| Dedup | ✅ exists | `bd find-duplicates` / `bd duplicates` |
| **ICE as a first-class field** | ❌ gap | only free-form metadata; no numeric sort/filter on `ice.score`, no auto-derived priority bucket |
| **Theme-coverage report** | ❌ gap | no single command for "themes with zero open beads"; today it's N `count` calls |

The two real gaps are ergonomic, not blocking. W0 can ship with metadata + shell
glue; native support is a fast-follow only if grooming volume makes the scripting
painful.

## 7. W0 requirements

The first implementation bead delivers the **thinnest runnable slice**: the
supervisor tick wiring (§5) plus the auto-decay sweep (§4), because decay is the
highest-value/lowest-cost tier and needs zero new tooling.

- **W0 scope:** `grooming_due` + swap-gate guard in the meta-orchestrator tick;
  the §4 decay query writing `needs-triage` + `decay.flagged_at`; one trail kind
  (`supervisor.meta.bead_decayed`); a `grooming_complete` summary line.
- **Out of scope for W0:** quarterly re-rank and ICE rescore automation (manual
  first, automate once the cadence proves out); native ICE field; theme-coverage
  command.
- **Done when:** a synthetic >90d open bead is flagged within one grooming session
  and appears in `bd list --label needs-triage`, with both trail lines present.

W0 deliberately leaves §1–§3 as supervisor playbook steps (run by hand against the
commands above) so the cadence can be validated before investing in automation.

---

## Consequences

**Positive:** grooming cost scales with how often each kind of drift actually
occurs; decay protects the backlog daily; almost all of it runs on today's `bd`
CLI; W0 is a few hours of tick wiring, not a tooling project.

**Negative / trade-offs:** three modulo periods are more bookkeeping than one
flat session, and ICE-in-metadata means no numeric sort until the native field
lands. Both are accepted: the periods are cheap arithmetic, and the metadata
workaround is good enough for the volumes [[ce-h59v]] targets (P0<20 / P1<100).
