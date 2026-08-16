---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-15
tokens: 360
title: Harness Hygiene
description: Earn every mechanism continuously; tell agents WHAT and WHY not HOW; pare over-fits now and consolidate over-harness later; never "delete because it's long".
tags: [policy, engineering, harness, over-fitting, simplicity]
---

# Harness Hygiene

The engine keeps getting smarter; the harness accretes crud one reasonable
correction at a time. Two distinct pathologies: **over-harness** (scaffolding
whose cost exceeds its value) and **over-fit** (a rigid point-fix welded to one
past incident). They have OPPOSITE urgency — split the timer.

## NEVER

- Tell an agent HOW to do its job when WHAT + WHY would do. A narrow set of
  broad, well-scoped tools beats many over-engineered ones with complex routing.
- Add a safety gate without a **liveness counter-check** — a crash-prevention
  control with no liveness check becomes a total-work-prevention control (a
  gate keyed on a macOS memory metric that reads near-zero forever never
  re-opened, deadlocking the pipeline ~2 days; full incident in
  [why](harness-hygiene.why.md#real-failure-cases-this-repo)).
- Defer paring an over-fit to a "later cleanup" bead. The later never comes; the
  RAM-gate bug was re-embedded THREE times while its cleanup sat deferred.
- Enumerate all bad cases in a denylist — invert to allowlist known-safe.
- Encode a binary requirement (JSON validity, a word cap, a threshold, a file's
  existence) as prose the model self-checks. Prose rots when the model changes.
- Pare by "delete because it's long". "Missing information is still missing
  information" — that is not a valid disposition.

## ALWAYS

- **Earn complexity continuously.** Build each capability to working, then give
  it the right job before moving on — not "build everything, pare later".
- **Page over-fits; defer over-harness.** Over-fits cause outages now — repair
  immediately. Over-harness consolidation (spawn builders, model-config sites,
  supervisor mesh) is real but not on fire — defer until the flywheel is stable.
- **Pare with the six verdicts, never ad hoc:** Keep · Give it one home · Load it
  later · Turn it into a check · Put it on probation · Retire it safely.
- **One rule, one home, one owner.** Other files CALL the canonical copy; every
  duplicate is a place for it to drift.
- **Hard requirements get hard checks** — a parser/counter/validator/scan, not a
  paragraph. See the `raw-mem-gate` scan in `cmd/structural-health` for the
  reference pattern.
- **Prefer one general mechanism to N per-incident ones** (config/registry over
  compiled constants; a shared helper over three hand-tuned thresholds).

## REMINDER

A fix is only "done" when it is (a) wired into a closed observation loop —
wire it or delete it — AND (b) carries a liveness counter-check so it cannot
strangle what it protects.
Ship nothing that fails either test. Don't let perfect be the enemy of good —
these are a review LENS, not a new gate.
