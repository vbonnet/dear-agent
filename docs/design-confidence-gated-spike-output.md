# Design Spike: Confidence-Gated Decision Function for Spike Output Quality

**Bead:** ce-90si · **Status:** Investigation only (no implementation) · **Date:** 2026-06-21

## Problem

Spike outputs vary in quality. A spike that evaluated two of five real options,
or whose evidence is a single skimmed file, looks structurally identical to a
thorough one — both produce a markdown doc with a recommendation. Today nothing
quantifies that gap, so a low-confidence spike can flow straight into
implementation and waste a worker on poorly-scoped work. We want a confidence
score that gates the spike→implementation handoff: high uncertainty should
**recurse** (deeper investigation or sub-spikes per [[ce-kky0]]), not proceed.

## 1. Confidence Score Definition

Score a spike output **0–100**, the sum of four weighted dimensions. Each
dimension is scored 0–4 against an anchored rubric, then weighted:

| Dimension | Weight | 0 (poor) | 4 (strong) |
|---|---|---|---|
| **Option completeness** | ×6 | 1 option, no alternatives | Plausible option space enumerated; exclusions justified |
| **Evidence strength** | ×7 | Assertion / single skim | Multiple primary sources read; claims cited to file:line or doc |
| **W0 specificity** | ×7 | "Build the thing" | Testable acceptance criteria, named files/schemas, scoped effort |
| **Unknown-unknowns** | ×5 | Risks unstated | Open questions explicit; residual uncertainty named & bounded |

Max = 4×(6+7+7+5) = **100**. The two heaviest dimensions (evidence, W0
specificity) are the ones that most directly predict whether implementation
succeeds. The unknown-unknowns dimension is deliberately *inverse-rewarded*: a
spike scores high by **admitting** what it does not know, not by appearing
certain — this is the primary anti-gaming lever (§5).

**Categorical bands** for human-facing display: **0–49 LOW** (recurse),
**50–74 MEDIUM** (gate-conditional), **75–100 HIGH** (pass).

## 2. Computation + Storage

**Three-stage scoring**, cheapest first, escalating only on disagreement:

1. **Worker self-score** (required). The spike worker emits its own rubric
   breakdown in the structured output (companion [[ce-t8kn]]). Cheap, but
   self-interested — treated as a *claim*, not a verdict.
2. **Orchestrator review** (required for MEDIUM/LOW, sampled ~20% for HIGH). An
   independent agent re-scores from the artifact alone. The **review score is
   authoritative**; a self-score >15 points above review flags a calibration
   problem (§5).
3. **Wayfinder record** (storage). Final score + per-dimension breakdown + scorer
   identity persist as a `confidence` block in wayfinder phase state, mirrored to
   a bead trail entry for audit.

**Storage:** bead metadata field `spike.confidence` (JSON: `{score, bands,
dimensions{}, scorer, ts}`) is the source of truth — queryable by supervisors
for backlog triage. The trail entry is the immutable audit copy. Avoid putting
the score only in free-form markdown; it must be machine-readable for the gate.

## 3. Gate Threshold + Calibration

**Default threshold: score ≥ 70 → PASS.** Below 70 → RECURSE.

Calibrate the threshold against *outcomes*, not intuition. Method:

1. Retro-score ~15–20 already-completed spike beads (e.g. ce-04cv repeat-back,
   ce-228u cross-agent learnings, ce-4h06 W0.5 gate) using the rubric.
2. Label each by realized outcome: did the implementation bead land cleanly, or
   did it bounce/re-spike/get abandoned?
3. Pick the threshold that maximizes separation — e.g. if clean-landing spikes
   cluster ≥72 and bounced ones ≤63, set the gate in the gap (70).
4. Re-calibrate quarterly as the sample grows; track the false-pass rate
   (passed-but-bounced) and false-recurse rate (recursed-but-was-fine).

Example separation from a 6-bead pilot (illustrative): clean landers scored
74/78/81/76; bouncers scored 58/61 — a clean gap at 70.

## 4. Interaction with the W0.5 Gate

The confidence score is the **engine**; the W0.5 gate ([[ce-4h06]]) is the
**checkpoint** that consumes it. W0.5 sits between spike output (W0 exit) and
implementation start. Division of labor:

- **W0.5 = policy.** Decides PASS / RECURSE / ESCALATE given the score, the
  band, and bead priority (a P3 may pass at 65; a P1 touching prod may demand
  80 + human sign-off).
- **Confidence function = measurement.** Produces the number W0.5 reads.

Recursion is bounded to **2 retries**; a third sub-threshold result auto-escalates
to a human rather than looping (shares the depth cap with [[ce-kky0]] sub-spike
decomposition). This keeps confidence-gating from becoming an infinite refinement
loop. W0.5 is the single place the score has teeth — elsewhere it is advisory
metadata for triage.

## 5. Failure Modes

- **Self-score inflation.** Worker scores itself 90 to skip review. Mitigation:
  review score authoritative; self-vs-review delta tracked per worker; large
  positive deltas degrade trust and force full review.
- **Rubric gaming / cargo-culting.** Worker pads option count or citation count
  to hit anchors. Mitigation: evidence dimension scored on *whether claims are
  supported*, not citation volume; reviewer spot-checks one cited claim.
- **Miscalibration drift.** A fixed 70 silently lets bad spikes through as task
  mix changes. Mitigation: quarterly outcome-based recalibration; alert if
  false-pass rate exceeds a target (e.g. >15%).
- **Confidence theater.** A confident-sounding doc scores high while being
  shallow. Mitigation: the unknown-unknowns dimension rewards admitting gaps;
  reviewer asks "what would change the recommendation?" — no answer caps the
  score.
- **Goodhart on the number.** Once gated, the score becomes a target. Mitigation:
  keep the rubric versioned and the gate threshold private to the policy layer;
  audit a random HIGH sample, not just LOW ones.

## 6. W0 Requirements for the Implementation Bead

1. Rubric encoded as a versioned config (`confidence_rubric.vN`), dimensions +
   weights + anchor text — not hardcoded.
2. Structured `spike.confidence` field on bead metadata (depends on [[ce-t8kn]]
   schema) + trail mirror; both written atomically at spike close.
3. Worker self-score step in the spike output protocol; orchestrator re-score
   step with delta tracking.
4. W0.5 gate ([[ce-4h06]]) reads the score and applies the PASS/RECURSE/ESCALATE
   policy with the 2-retry cap.
5. A retro-scoring harness over historical spike beads to seed the initial
   threshold, plus a calibration report (false-pass / false-recurse rates).
6. Acceptance: a sub-threshold spike provably cannot open an implementation bead
   without recurse-or-escalate; calibration report shows the gap-based threshold.

## Companion Beads

[[ce-4h06]] W0.5 gate · [[ce-t8kn]] structured output schema ·
[[ce-kky0]] recursive decomposition · [[ce-ynyb]] spike pattern adoption.
