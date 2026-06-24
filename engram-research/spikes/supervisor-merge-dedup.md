# Spike: Explicit overlap dedup in the supervisor merge phase

**Bead:** ce-emnl
**Status:** Design / research (no implementation in this bead)
**Author:** vroom worker `ce-emnl`
**Scope:** Make the supervisor's merge of parallel-worker output an *auditable*
step. Today overlap is dropped silently; this spike defines a protocol, a trail
schema, and dedup rules so every "did NOT merge" decision leaves a record.

---

## 1. The problem

The VROOM orchestrator (`vroom-orchestrator`, see
`cmd/vroom-dispatch/skills/orchestrator.md`) dispatches multiple worker beads in
parallel for a single roadmap item. Each worker proposes changes — a diff, a
config block, a doc edit, a bead note. When those workers finish, their output
has to be reconciled into a single coherent result before the parent bead is
closed.

Right now that reconciliation is implicit:

- **Overlap is dropped silently.** If worker A and worker B both touch the same
  file, or propose the same constraint block, whichever lands first wins and the
  other is effectively discarded. Nothing records *that* it was discarded, let
  alone *why*.
- **No audit trail of the negative space.** The trail
  (`~/.agm/vroom/trail.jsonl`) captures what was dispatched
  (`supervisor.orch.dispatched`) and what was nudged/killed/died, but there is
  no record of what a merge *rejected*. The history shows what we kept; it is
  blind to what we deliberately threw away.
- **Closures hide contradictions.** A multi-worker bead is closed once its PR is
  merged. If two workers proposed *opposite* changes and the supervisor picked
  one, the rejected branch of that decision vanishes. A later reviewer (human or
  the Overseer's closure audit) cannot tell whether the contradiction was
  *resolved* or simply *missed*.

The cost is paid later: duplicated effort re-discovered in retros, "why didn't we
do X?" questions with no answer on record, and silent regressions when a
rejected-but-correct proposal turns out to have been the right one. The DEAR
retro already found beads closed against unmerged PRs (ce-6f1b, ce-mcw2,
ce-1onr); the merge phase is the same class of problem one level up — a
state-transition that isn't backed by an explicit, inspectable record.

**Goal of this spike:** every proposal that a merge does *not* accept must leave
a record stating which proposal it was and why it was not accepted.

---

## 2. Proposed merge phase protocol

When the workers for a parent bead have all finished, the supervisor runs an
explicit **Merge Phase** that classifies every worker proposal into exactly one
of four buckets. The phase is a pure bookkeeping pass over the set of completed
worker proposals; it produces a structured decision, not just a merged diff.

| Bucket | Meaning | Required annotation |
|--------|---------|---------------------|
| **KEPT** | Proposal accepted into the merged result | brief rationale (what it adds) |
| **REJECTED** | Proposal not accepted | explicit reason — one of: `duplicate`, `contradicts-constraint`, `interface-mismatch`, `out-of-scope` |
| **CONFLICTS** | Two proposals contradict each other | the contradiction + the resolution decision (which side won, why) |
| **DEFERRED** | Proposal is valid but out of scope for this merge cycle | what it is + where it should be re-filed (follow-up bead) |

Rules of the protocol:

1. **Total coverage.** Every worker proposal lands in exactly one bucket. A
   proposal that is silently absent from all four buckets is a protocol
   violation — the merge is not complete until every proposal is accounted for.
2. **Reasons are enumerated, not free-text-only.** REJECTED entries carry a
   machine-readable reason code (the four above) *plus* a human sentence. This
   keeps the trail queryable ("show me everything rejected as `duplicate` last
   week") while preserving the narrative.
3. **CONFLICTS is not REJECTED.** A conflict is a *pair* of proposals where
   choosing one rejects the other. It is logged distinctly because it carries
   information REJECTED does not: there was a genuine disagreement, and a
   resolution was made. The losing side of a conflict is recorded inside the
   conflict entry, not duplicated into REJECTED.
4. **DEFERRED creates a follow-up.** A DEFERRED proposal must name (or create) a
   follow-up bead so the deferred work is not lost. Deferral without a landing
   spot is just a slower silent drop.

---

## 3. Implementation in dear-agent

Three coordinated touch points. None require new Go subsystems — the merge phase
is supervisor *instructions* plus a new trail `kind` plus a bead-note
convention, mirroring how dispatch already works today.

### 3a. `cmd/vroom-dispatch/skills/orchestrator.md` — add a "Merge Phase" step

The orchestrator skill is a tick loop (Steps 1–N, each tick ~90s). Today
worker-completion handling lives in **Step 7: Monitor Active Workers** and the
closure rules under the bead-lifecycle section (a bead is Done only when its PR
is MERGED). Insert a **Merge Phase** step that fires when *all* workers for a
multi-worker parent bead have completed and before the parent bead is closed:

> **Step (Merge Phase): reconcile completed multi-worker output.**
> When every worker dispatched for parent bead `<P>` has finished (PR merged or
> proposal delivered), classify each worker's proposal into KEPT / REJECTED /
> CONFLICTS / DEFERRED per the dedup rules below. Emit one
> `supervisor.merge.decision` trail record covering the whole parent bead, and
> write a structured merge summary into `<P>`'s bead note before closing it. Do
> not close `<P>` until the merge decision record exists.

This is consistent with the skill's existing pattern: a guarded step that
records a `supervisor.*` trail `kind` and gates a state transition (here:
closing the parent bead), exactly like the "closed dependency must be backed by
a merged PR" guard already does for dispatch.

### 3b. Trail entries — new kind `supervisor.merge.decision`

Add a new trail `kind`. Trail records follow the existing schema (see
`writeTrail` in `cmd/vroom-dispatch/main.go` and the `supervisor.orch.dispatched`
example in the orchestrator skill): top-level `ts`, `role`, `kind`, `payload`.
The four buckets live under `payload`:

```
{ "ts": ..., "role": "orchestrator", "kind": "supervisor.merge.decision",
  "payload": { "parent_bead": ..., "accepted": [...], "rejected": [...],
               "conflicts": [...], "deferred": [...] } }
```

`accepted` / `rejected` / `deferred` are arrays of `"<bead-id>: <reason>"`
strings; `conflicts` is an array of objects `{between, resolution}`. The record
is append-only to `~/.agm/vroom/trail.jsonl`, same file and same atomic-append
discipline as every other supervisor trail record. Because `kind` is just a
string and `payload` is an open `map[string]any`, **no Go schema change is
required** — the new kind is emitted by the supervisor `printf`-ing the record,
identical to how `supervisor.orch.dispatched` is emitted today.

### 3c. Bead notes — structured merge summary on close

When the orchestrator closes a multi-worker parent bead, it writes a structured
merge summary as a bead note (always
`bd --db ~/beads/context-engine/.beads note <P> "..."`). The note is the
human-facing mirror of the trail record — the trail is for machine queries and
the Overseer's audit; the bead note is what a human reviewing the bead's history
sees. Format:

```
MERGE (parent <P>, N workers):
  KEPT:     ce-abc — add GlobalConstraints block
  REJECTED: ce-def — duplicate of ce-abc
            ce-ghi — contradicts constraint X
  CONFLICT: ce-jjj vs ce-kkk — chose ce-jjj (P1 > P2); see trail
  DEFERRED: ce-jkl — valid, out of scope this sprint → follow-up ce-zzz
```

This pairs with the existing closure discipline (verify PR MERGED before
`bd ... close`): the merge summary is written in the same close step, so a closed
multi-worker bead always carries its merge rationale.

---

## 4. Dedup rules

The Merge Phase classifies each pair of overlapping proposals using three rules.
Overlap is detected by *target* (same file, same config block, same bead field)
and *intent* (same change vs. opposite change).

### Exact duplicate — same file + same change
Two proposals make the *same* change to the *same* target. Keep exactly one;
classify the other REJECTED with reason `duplicate`.
> log: `ce-def: duplicate of ce-abc`

### Partial overlap — overlapping but not identical changes
Two proposals touch the same target with *different but compatible* approaches
(e.g. both add fields to the same block, or one is a superset of the other).
Make an explicit merge decision: either combine them (both KEPT, with a note on
how they were merged) or pick the stronger approach (one KEPT, one REJECTED with
reason `interface-mismatch` and a rationale).
> log: `worker A's approach chosen over B because A handles the nil case; B folded in`

### Contradiction — opposite changes to the same target
Two proposals make *mutually exclusive* changes (A sets X=true, B sets X=false).
This is a CONFLICTS entry, resolved by priority:
- **P0 contradiction → human-escalate.** Do not auto-resolve a P0 disagreement.
  Send an urgent message to the Overseer / file for human review; record the
  conflict as unresolved-pending-human in the trail. (The orchestrator is
  pre-authorized to *dispatch* autonomously, but a P0 contradiction is exactly
  the kind of correctness fork that warrants a human, matching the existing
  "P0 / blocking work" special-casing in the skill.)
- **P1 or lower → reject the lower-priority side.** Keep the higher-priority
  proposal, REJECT the other inside the conflict's `resolution`, and record why.
  Ties broken by worker dispatch order (earlier-dispatched wins) and noted as
  such.

> log (P1+): `conflict ce-jjj vs ce-kkk on constraint X — kept ce-jjj (P1), rejected ce-kkk (P2)`
> log (P0):  `conflict ce-jjj vs ce-kkk on constraint X — P0, escalated to human, merge held`

---

## 5. Example merge decision trail entry

The task-level shape (flat, illustrative):

```json
{"kind": "supervisor.merge.decision", "ts": "...",
 "accepted": ["ce-abc: add GlobalConstraints block"],
 "rejected": ["ce-def: duplicate of ce-abc", "ce-ghi: contradicts constraint X"],
 "deferred": ["ce-jkl: valid but out of scope for this sprint"]}
```

The wire form actually written to `~/.agm/vroom/trail.jsonl`, aligned with the
existing `{ts, role, kind, payload}` schema (`writeTrail`,
`cmd/vroom-dispatch/main.go`) so it sorts and queries alongside every other
supervisor record:

```json
{"ts":"2026-06-21T23:14:07Z","role":"orchestrator","kind":"supervisor.merge.decision","payload":{"parent_bead":"ce-par1","workers":4,"accepted":["ce-abc: add GlobalConstraints block"],"rejected":["ce-def: duplicate of ce-abc","ce-ghi: contradicts constraint X (interface-mismatch)"],"conflicts":[{"between":["ce-jjj","ce-kkk"],"resolution":"kept ce-jjj (P1) over ce-kkk (P2) on constraint X"}],"deferred":["ce-jkl: valid but out of scope this sprint -> follow-up ce-zzz"]}}
```

Emitted exactly like the dispatch record already is:

```bash
printf '{"ts":"%s","role":"orchestrator","kind":"supervisor.merge.decision","payload":%s}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$MERGE_PAYLOAD_JSON" \
  >> ~/.agm/vroom/trail.jsonl
```

---

## 6. Why this is enough (and what it deliberately is not)

- **No new Go subsystem.** The merge phase reuses the trail file, the open
  `payload` map, and the bead-note CLI. The change is supervisor instructions +
  one new `kind` string + a note convention — the lowest-friction path that still
  produces a durable, queryable record.
- **Audit-friendly.** The Overseer already audits closures (beads closed against
  unmerged PRs). A `supervisor.merge.decision` record per multi-worker close
  gives that audit a second invariant to check: *every closed multi-worker bead
  has a merge decision, and every worker proposal is accounted for in it.*
- **It is not automatic conflict resolution.** The rules pick a winner for
  P1+ overlaps mechanically, but the *value* is the record, not the automation.
  A wrong auto-resolution that is logged is recoverable; a silent drop is not.
- **Out of scope for this spike:** detecting overlap programmatically (the
  supervisor reasons about overlap from worker output today), a UI over the
  decision records, and retro-active backfill of past merges. Those are
  follow-ups once the record format is in place.

## 7. Follow-ups

1. Implement the Merge Phase step in `orchestrator.md` (skill-only change).
2. Add the `supervisor.merge.decision` example to the trail-kind reference so the
   Overseer audit can assert on it.
3. Teach the Overseer closure audit the new invariant (multi-worker close ⇒
   merge decision record exists).
4. Decide whether DEFERRED auto-creates the follow-up bead or just names it.
