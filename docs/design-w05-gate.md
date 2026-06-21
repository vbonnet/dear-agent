# Design Spike: W0.5 Conditional Gate Between Spike Output and Implementation

**Bead:** ce-4h06 · **Status:** Investigation only (no implementation) · **Date:** 2026-06-21

## Problem

A spike produces a requirements/design doc and hands off to a *separate*
implementation bead. Nothing inspects that doc before implementation starts, so a
spike that evaluated two of five options — or cited a single skimmed file — opens
an implementation bead that looks identical to one backed by a thorough spike.
Worker time is then spent building poorly-scoped work. We want a **W0.5 gate**: a
conditional checkpoint that consumes the confidence score ([[ce-90si]],
`docs/design-confidence-gated-spike-output.md`) and decides whether the handoff
may proceed.

## 1. Gate Placement

The "W0.5" name is logical, not a tenth phase. In the canonical V2 model
(`CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO`; see
`wayfinder/PHASES.md`), a spike session runs `CHARTER`→`SPEC` and stops — its
deliverable is the requirements doc, the analogue of V1's `W0`+`D1–D4`
artifacts. Implementation is the `PLAN`→`BUILD` tail, almost always in a
**different bead**.

W0.5 therefore sits at the **discovery→implementation boundary: SPEC exit, before
an implementation bead may open** (V1: after the `D4` requirements deliverable,
before `S6`). Concretely it is a **spike-close hook**, not an in-session phase
transition — it fires when a spike bead attempts to close-and-spawn, gating bead
creation rather than a `complete-phase` call. This mirrors the existing
discovery-boundary gates (`wayfinder/lib/d1-gate-check.sh`,
`d2-gate-check.sh`) and the BUILD-exit gates (WAY-15/16/17 in
`wayfinder/SPEC.md`), reusing the established "read the deliverable, emit a
verdict, optionally escalate" pattern.

## 2. Pass Criteria

W0.5 is **policy over measurement**: the confidence function produces the number,
W0.5 decides what the number buys. PASS requires **all** of:

1. **Minimum fields present.** The structured `spike.confidence` block exists and
   is well-formed (score, bands, per-dimension breakdown, scorer, ts), and the
   requirements doc carries the mandatory sections: options evaluated, evidence,
   W0-requirements-for-implementation, open questions.
2. **Score threshold by priority.** Default `score ≥ 70` (the ce-90si gate). The
   threshold is priority-scaled, not fixed: a `P3` may pass at 65; a `P1`/prod or
   security-touching bead demands `≥ 80` **and human sign-off** (reuse the
   d1-gate-check auth/security keyword detector to trigger the stricter band).
3. **Calibration sanity.** Self-vs-review delta within tolerance (≤15 pts per
   ce-90si §5); a large positive delta forces full review before PASS.

MEDIUM band (50–74) is **gate-conditional**: PASS only if priority is low *and*
no security keywords fire; otherwise treated as FAIL. Human sign-off is required
only in the high-priority/security path, not for every PASS.

## 3. Fail Handling

On FAIL, W0.5 emits one of three verdicts (never silently blocks):

- **RECURSE** (default for LOW/borderline): re-open the spike for deeper
  investigation, annotated with the *failing dimensions* so the next pass is
  targeted. Bounded to **2 retries** (shared depth cap with ce-90si and
  [[ce-kky0]]).
- **DECOMPOSE**: when failure is driven by *scope* (multiple independent unknowns,
  low option-completeness across distinct areas), spawn sub-spikes via the
  [[ce-kky0]] pattern and synthesize before re-gating, rather than re-running one
  oversized spike.
- **ESCALATE**: a third sub-threshold result, an unresolvable self-vs-review
  conflict, or P1/prod failure auto-escalates to a human. This is the loop
  backstop — W0.5 must never become an infinite refinement cycle. Deferral to a
  later priority (P2) is a human decision made at escalation, not an automatic
  verdict.

## 4. Wayfinder Schema Changes

- **Gate registration.** Add `w05-gate-check.sh` to `wayfinder/lib/` following
  the existing gate-check contract (reads deliverable path, exits 0/non-zero,
  prints verdict). Wire it into the spike-close path rather than a phase
  transition.
- **Phase/session state.** Extend wayfinder phase state with a `w05` block
  (`{verdict, threshold_applied, retries_used, signoff}`) alongside the ce-90si
  `confidence` block; mirror to a bead trail entry for audit.
- **Policy config.** A versioned `w05_policy.vN` (priority→threshold table,
  retry cap, security-keyword→sign-off rule) — not hardcoded, paralleling the
  versioned `confidence_rubric.vN`.
- **Bead linkage.** A `spawned_from`/`gate_verdict` field on the implementation
  bead so a passed gate is provable and queryable; an implementation bead with no
  passing W0.5 verdict is invalid.

## 5. Integration with Companion Spikes

W0.5 is the **single checkpoint with teeth**; the companions supply its inputs.

- **ce-90si (confidence).** The engine. W0.5 reads `spike.confidence` and applies
  policy. Elsewhere the score is advisory triage metadata; only at W0.5 does it
  block. The 70-default threshold and 2-retry cap originate there.
- **ce-kky0 (recursive decomposition).** The DECOMPOSE verdict *is* the ce-kky0
  trigger. They share the recursion-depth cap so confidence-recurse and
  scope-decompose cannot independently blow past it. Sub-spike synthesis must
  re-enter W0.5 before the parent can hand off.

## 6. W0 Requirements for the Implementation Bead

1. `w05-gate-check.sh` honoring the gate-check contract and wired into
   spike-close (not phase transition).
2. Versioned `w05_policy.vN`: priority→threshold table, retry cap, security
   sign-off rule.
3. Session-state `w05` block + trail mirror, written atomically at gate
   evaluation.
4. PASS/RECURSE/DECOMPOSE/ESCALATE state machine with the shared 2-retry cap and
   human-escalation backstop.
5. Bead linkage proving an implementation bead cannot open without a passing
   verdict.
6. Acceptance: a sub-threshold spike provably cannot spawn an implementation
   bead without recurse/decompose/escalate; high-priority FAILs route to human
   sign-off; retries are bounded.

## Companion Beads

[[ce-90si]] confidence-gated decision function ·
[[ce-kky0]] recursive spike decomposition ·
[[ce-t8kn]] structured output schema.
