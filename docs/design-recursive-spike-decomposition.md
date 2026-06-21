# Design: Recursive Spike Decomposition

**Status:** Spike / investigation (ce-kky0). No implementation.
**Problem:** Some investigations are too large or too uncertain for a single
spike. Today there is no pattern for splitting a spike into sub-spikes, nor for
synthesizing their outputs back into one parent recommendation. Without one, an
over-scoped spike either sprawls past its time box or ships a low-confidence
recommendation that the W0.5 gate ([[ce-4h06]]) will bounce anyway.

This doc proposes decomposition triggers, parent/child linking, a synthesis
owner, a recursion depth limit, and the W0 hand-off for an implementation bead.

## 1. Decomposition triggers

A spike SHOULD decompose when any one trigger fires. The first three are signals
the *worker* raises; the fourth is enforced mechanically.

| Trigger | Concrete signal |
|---------|-----------------|
| **Scope too large** | The spike enumerates ≥3 independent unknowns that do not share evidence (each would be researched separately anyway). |
| **Confidence below threshold** | The confidence score from [[ce-90si]] lands below the recurse threshold (proposed < 0.5) at the time box's midpoint. |
| **Multiple independent unknowns** | The recommendation depends on ≥2 questions whose answers don't constrain each other (e.g. "which transport?" and "which storage?"). |
| **Time overrun** | Elapsed time exceeds 1.5× the spike's estimate with no draft recommendation. A hard, observable trip-wire so a stuck spike decomposes instead of silently sprawling. |

One trigger is sufficient. Decomposition is itself a decision: the worker records
which trigger fired in a trail entry before spawning children, so the choice is
auditable and not re-litigated on resume.

## 2. Linking sub-spikes to the parent

Sub-spikes are first-class beads, created with the **`bd` parent field** set to
the parent spike. This gives a queryable tree (`bd show <parent>` lists children)
without inventing new schema.

- **Parent field** — structural containment; the one source of truth for the tree.
- **`bd link`** — a `decomposes` / `blocks` edge from each child back to the
  parent so the parent cannot close while children are open. Reuses the existing
  dependency graph the scheduler already honours.
- **Wayfinder phase** — children inherit the parent's roadmap and run their own
  W0→W1 sub-sequence. The parent's W0.5 gate is deferred until synthesis.
- **Trail entries** — the parent logs one entry per spawn (`spawned ce-xxxx:
  <unknown>`) and one on synthesis, so the decomposition is reconstructable from
  the trail alone.

Children carry the labels `spike, sub-spike` plus the parent id, so a dropped or
orphaned child is greppable.

## 3. Synthesis

**Owner: a dedicated synthesis spike, created as the last child of the parent.**
It `depends_on` every sibling sub-spike, so the scheduler runs it only after all
children resolve. It reads the children's recommendation docs and produces the
parent's single recommendation plus a merged confidence score.

Rejected alternatives:
- *Supervisor aggregation* — couples synthesis to orchestration; supervisors
  shouldn't make domain calls.
- *Automated merge* — sub-spike outputs are prose recommendations with trade-offs;
  mechanical concatenation loses the judgment that justifies a spike at all.
- *Orchestrator pass* — same coupling problem, and no audit trail.

A dedicated bead keeps synthesis observable, time-boxed, and itself subject to the
W0.5 gate. The synthesis spike is the **only** node that emits the parent's W0.5
artifact; children feed it, they do not gate independently.

## 4. Recursion depth limit

**Maximum depth: 2** (parent → child → grandchild). Rationale:

- Depth 0 is the original spike; depth 1 splits independent unknowns; depth 2
  covers the rare case where one unknown itself fractures. Empirically, real
  investigations bottom out by the second split — beyond that the leaves are
  small enough to answer directly, not spike.
- Each level multiplies worker and synthesis cost; unbounded recursion is a
  runaway-decomposition risk (the same class of failure as a fork bomb).
- A hard cap is mechanically checkable: a `pretool` guard rejects a `bd create`
  whose computed parent-chain depth would exceed 2, forcing the worker to answer
  the leaf inline instead. At the cap, the trigger conditions are ignored —
  decomposition is simply unavailable.

Depth is computed from the parent chain, not a stored counter, so it can't drift.

## 5. Interaction with companion spikes

- **W0.5 gate ([[ce-4h06]])** — fires once, on the **synthesis spike's** output,
  never on individual children. A child that itself fails its own confidence check
  recurses (subject to the depth cap); it does not reach W0.5. This keeps a single
  gate between the *parent* recommendation and implementation.
- **Confidence-gated decision function ([[ce-90si]])** — supplies both the
  *trigger* (a low child score is a decompose signal) and the *synthesis output*
  (the merged score). The parent score is the **minimum** of child scores, not the
  mean — a single weak leg should keep the whole recommendation out of
  implementation. Decompose-on-low-confidence and gate-on-low-confidence share one
  threshold so the two mechanisms can't disagree.

## 6. W0 requirements for the implementation bead

An implementation bead (W0) following this spike must deliver:

1. **`bd` schema/CLI**: confirm the parent field + `decomposes` link type exist;
   add them if not. `bd show` must render the sub-spike tree.
2. **Depth guard**: a `pretool-bead-create-guard` extension that rejects spike
   beads whose parent-chain depth would exceed 2, with a `--reason` override path.
3. **Synthesis-spike template**: a bead template that auto-sets `depends_on` for
   all siblings and carries the synthesis runbook.
4. **Wayfinder wiring**: defer the parent W0.5 gate until the synthesis child
   resolves; children run isolated W0→W1 sub-sequences.
5. **Score propagation**: parent confidence = min(child scores); store on the
   synthesis bead's metadata and trail.
6. **Tests**: depth-cap rejection, parent-cannot-close-with-open-children,
   synthesis-runs-last ordering.

Scope is process plumbing over the existing bead/wayfinder machinery — no new
storage tier and no new gate beyond the one W0.5 already defines.
