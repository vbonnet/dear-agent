# dear-agent strategic priorities

```
as_of:          2026-08-18
owner:          vbonnet
beads_db:       ~/beads/context-engine/.beads (Beads is the task source of truth)
beads_snapshot: 1769 total; 968 active (823 open, 145 in progress); 72 open P0, 305 open P1
next_review:    2026-08-25 (weekly reconciliation)
supersedes:     engram-research/ROADMAP-FLYWHEEL-2026-06-10.md (stale 267-bead baseline)
refines:        engram-research PR #322 (strategy/2026-08-14-dear-agent-strategic-state.md),
                engram-research/plans/2026-07-26-dear-agent-strategic-flywheel-review.md
```

This file is the canonical current prioritization, refined 2026-08-18 from the existing
roadmap artifacts rather than rewritten. It implements the canonical-store proposal in
engram-research PR #322 Part 3 (location dear-agent root as proposed; named
STRATEGIC-PRIORITIES.md, the operator's other named option, because the repo's
Living Documentation Policy forbids a tracked ROADMAP.* here and the routing-guard
test enforces it). Authority model, unchanged from PR #322: if stores
disagree, task status comes from Beads and delivery state comes from live receipts;
this file must be corrected and never overrides either. Bound: 250 lines or 20
active initiatives. Every rank change cites its evidence in the changes section.

## North star (unchanged)

A recursive self-improving harness: sessions emit traces, traces become retros and
eval cases, evals gate changes, benchmarks measure the harness, automated research
proposes changes, winners merge. (ROADMAP-FLYWHEEL-2026-06-10, kept verbatim.)

What changed is the route, not the destination: the week of 2026-08-11..18 proved
the binding constraint is continuous-execution friction, not a shortage of ideas.
The mesh currently lacks four guarantees: spawns succeed or fail loudly, credentials
and gates do not self-destruct on a timer, finished work reaches a live consumer, and
every fail-closed component proves liveness. Now items 1 through 4 fund exactly those
four guarantees before any new capacity.
(Source: engram-research planning/2026-08-18-continuous-execution-friction-priorities.md,
closing thesis; seven 2026-08-18 retros; PR #327.)

## Now (maximum 5)

**N1. Spawn integrity: spawns succeed or fail loudly.**
Beads: ce-gdgld, ce-o01ps, ce-q4bgk, ce-cm8q0, ce-fmxv, ce-2mib, ce-r6sem, ce-d1jw, ce-wn4qe.
Outcome: close the blocking-modal class (seed trust and onboarding state at session
create, version-pinned TTY probe asserting prompt-ready with zero modals), fix the
circuit breaker population (registry sessions, not tmux panes; route MCP
agm_create_session through the same admission point; fail closed on probe error),
and make failed spawns keep a spawn_failed record instead of vanishing.
Receipts: wizard fix verified 08-17 (ce-o01ps) but only the instance; trust dialog
recurrence filed 08-18 (ce-gdgld).
Evidence: 12/32 sessions wedged on a modal 08-17; 47/54 overnight dispatch ticks
deadlocked on ghost counts; 582/586 dirs un-onboarded; sessions rolled back with
records deleted, destroying diagnosis evidence (retros automode-wizard-spawn-wedge,
2026-08-18-trust-dialog-spawn-wedge, 2026-08-18-spawn-lifecycle-integrity).
Kill or reconsider: 2026-09-08 if two weeks pass with zero modal wedges.

**N2. Credentials and gates must not self-destruct on a timer.**
Beads: ce-o8oli, ce-sjgxo, ce-4f6aq, ce-77ip (epic, L follow-on), ce-5bjbn, ce-x5psr, ce-lz9ny, ce-1hu9.71.
Outcome: merge and deploy the OAuth fix (#1234 merged, deploy receipt required; main
shipped the -force revoker), alarm on any new token-family-death marker, finish
diff-scoping required checks (#1241/#1242/#1243), 7-day pre-sunset waiver alarm plus
exceptions CLI.
Evidence: 56 OAuth family deaths in window (24 on 08-14 alone), 414 markers total;
two queue-wide CI outages in five days blocked ~20 PRs including the OAuth fix itself
for 29h; next waiver sunset fires mid-September without the alarm (retros
2026-08-18-oauth-family-death-and-skew, 2026-08-18-required-check-timebombs).
Kill or reconsider: 2026-09-15, which is after the next sunset must have been survived.

**N3. Results reach a live consumer.**
Beads: ce-uwll9.1/.2/.3, ce-tyip7, ce-omvwv, ce-rk3v.1, ce-08y6, ce-0zng9 (deploy receipt).
Outcome: verify #1212 is live in the installed binary and launchd daemon (merged is
not deployed), land live-dispatcher discovery and the durable alert queue (#1224,
#1226), add relay-target liveness to agm doctor, then the pipeline-integrity test:
a worker finishing must reach the operator with no polling.
Evidence: 83 relay attempts went to a supervisor dead for 24 days; state writes
silently dropped while returning success; watchdog alerts have zero machine
consumers, so the flywheel is structurally blind (retros
2026-08-11-results-surfacing-blindness, 2026-08-18-notification-relay-dead-letter).
Kill or reconsider: 2026-09-08.

**N4. Every fail-closed component proves liveness.**
Beads: ce-2a3ma, ce-ja1f, ce-93lw.23, ce-pf91f, ce-t9zwt.
Outcome: heartbeats on every reaper and scheduled job, launchd nonzero-last-exit
sweep, watchdog-of-the-watchdog, sandbox-gc actually completing (#1160 open;
DOLT_PORT failure persisted through 08-18 despite #1233 merging 08-17), golden-clone
divergence alarm and reconcile of the 7 diverged repos.
Evidence: five silent-dead-component instances in six weeks (sandbox-gc 190+
consecutive exit-1 runs, dead relay, OAuth error wall, dead ci-health-monitor,
30-day red scheduled workflows); agents plan against stale goldens, which corrupted
this very session's inputs (retros 2026-08-07-recurring-disk-ballooning,
2026-08-18-required-check-timebombs; friction items 9 and 10).
Kill or reconsider: 2026-09-15.

**N5. Priority re-adjudication: P0 bankruptcy, round two.**
Beads: the adjudication itself; consolidate exact duplicates (ce-nq2r = ce-6oj2),
close stale deadline work (ce-t8rzf, deadline 2026-08-09 already past).
Outcome: re-adjudicate all 473 active P0/P1 items; cap the admitted P0 portfolio at
5 and P1 at 15 (PR #322 agenda item 1); everything else demoted, merged, or closed.
Evidence: 49% of the active queue claims P0/P1, so priority has stopped ordering
anything; the 07-26 review ordered one triage pass at 37 P0s and it never ran; open
P0s have since doubled to 72 (PR #322 Part 1; plans/2026-07-26 review, Part B).
Kill or reconsider: this is a bounded act, not a program; done means the caps hold
on 2026-08-25.

## Next (maximum 10, ordered)

**X1. Bounded bot-review loop (the backstop bound).** Delta-scoped re-review after
round 1, severity floor on required conversation resolution, round budget of 4 with
human escalation, land or de-require the keyless gate (#1221). Must not regress to
ce-lr7j severity-blind auto-resolve. Reframe (operator, 2026-08-18): repeated review
rounds on real bugs are working as intended; review is a backstop, not the learning
mechanism. X1 bounds the backstop; X2 is the loop that shrinks the need for it.
Evidence: #1096 died after 28 rounds/51 threads; #1243 hit 76 threads in under
2 days; dominant per-PR latency (retro 2026-08-18-unbounded-bot-review-loop; retro
2026-08-11-dear-agent-pr-merge-latency).
**X2. Review-comment mining: recurring review findings become just-in-time
instructions.** New first-class self-improvement mechanism (operator reframe,
2026-08-18). Progressively mine every automated-review comment, detect
recurring-mistake patterns, and convert each pattern into an instruction placed at
the right level, in strict preference order: task-specific agent core instructions,
then the relevant directory's AGENTS.md, then SKILL.md, then deterministic hook
injection at the moment of action. Never top-level AGENTS.md unless the lesson is
truly universal, to avoid context rot, token cost, and hallucination pressure;
prefer deterministic injection at the right moment over always-loaded prose. This is
the dreaming pattern of X9 applied to review data, and unlike X8 its input corpus
already exists in volume (51 threads on #1096, 76 on #1243), so it can start before
the trace sensors wake. North-star metric: automated-review catches per PR trending
toward zero, meaning agents produce correct PRs at creation. Pairs with the
failure-dossier flywheel (X8) and the instruction-placement work (persona-system,
agent-as-directory investigations in engram-research). Bead: none yet; file at
admission, citing this reframe.
**X3. Remediation lane through the guard stack.** Config-guard JSON parsing fix
(ce-l9iue) via the strict REVIEW.md gate, baseline-aware dotfiles preflight
(ce-6094z), write-guard deploy targets for declared LaunchAgents artifacts, audited
safe-pr waiver lane. Security boundaries stay; exceptions become bounded and audited.
Evidence: a verified P0 fix could not be PR'd this week; the repair path is gated
harder than the failure path, four instances in one week (retro
2026-08-18-remediation-path-gated).
**X4. Quota meter and tightest-window accounting.** ce-3rmqx.1/.2/.3; #1197, #1218,
#1223; burn-rate alarm; routing policy written and default. Evidence: quota
exhaustion is a fleet stop whose first signal is a request failure; sub-budget
exhausted before the weekly (retro 2026-08-11-multi-provider-quota-strategy).
**X5. Trustworthy local preflight.** Skip the macOS-broken WRONG_HARNESS test locally
as CI does; isolate test estates so fixtures stop polluting the breaker count.
ce-5w2hq. Evidence: ~40 test failures misdiagnosed as a runtime regression this week.
**X6. Claimed-vs-verified gates.** ce-hj7eb.1 (receipts required, machine-checked)
and ce-hj7eb.2 (no improvement claim without n, interval, baseline). Sequenced after
N3 so the pipeline the receipts flow through is itself truthful. Evidence: four
fabricated-or-inflated done reports in one day, none caught by a gate (retro
2026-08-11-claimed-vs-verified-slop).
**X7. Reserved supervisor capacity, load shedding, starvation dashboard.** Factorio
shortlist items 1 and 2: untouchable reserves for orchestration, health, recovery,
receipts; named signals with pre-dispatch floors. Evidence: the wizard incident
saturated the process cap with its own zombies, so healthy recovery spawns were
refused (research/dear-agent-factorio-analogies-2026-08-15, PR #321; wizard retro G6).
**X8. Trace-mining flywheel increments, explicitly gated.** ce-asrxq.1 after
ce-9xotf.3 (dossier schema and prevalence stats) and ce-qf0f (schedule the dormant
sensors; signals.db is empty); then ce-qrwt3.1 (counterfactual replay, 20 decision
points, 2 candidate models, sharing one normalized trace digest, after X4 because
substitution decisions need real cost data). Encode the prerequisites as real Beads
dependencies; today both epics show READY while their own descriptions name unmet
prerequisites. Evidence: bd dep tree shows no blockers; the miner would consume
exactly the session data N1 and N3 currently make unreliable (beads ce-asrxq,
ce-qrwt3; PR #322 cluster notes; dreaming plan 3c).
**X9. Dreaming: manual dream run plus memory CAS.** ce-9xotf.2 (one manual run
producing a consolidation PR; MEMORY.md is over its load limit today) then ce-9xotf.1
(compare-and-swap in the engram write path, failing two-writer test first). Days of
work, zero infra, immediate payoff (research/2026-08-17-dreaming-memory-curation-plan.md
priority order; RESEARCH-LEARNING-WHILE-YOU-SLEEP.md).
**X10. Belt-to-train graduation policy and byproduct queue accounting.** Factorio
items 3 and 7: explicit thresholds before a manual workflow earns a scheduled route,
and PRs awaiting review, sessions awaiting archival, branches awaiting closure
tracked as real WIP. Evidence: 72 of 130 leaked branches never had a PR; content was
silently re-solved by later sessions, so the system duplicates rather than loses work
(retro 2026-08-17-dear-agent-branch-leak-dear-retro; PR #321).

## Not now (with reactivation conditions)

- Pokemon TCG blunder-finder (ce-nsuo8): the Kaggle deadline (ce-t8rzf, 2026-08-09)
  passed while open. Reactivate when a live competition target exists.
- New strategy programs of any kind: PR #322 item 5 stands. No new program admission
  until N5 completes and holds one review cycle.
- Semantic cache, RAG modernization, progressive disclosure (old ROADMAP.md phases
  B-D): superseded since 2026-06-12, unchanged here.
- Research-epic freeze from the 07-26 review (ce-okcw.*, ce-kh70.*, ce-lafa.*):
  retained, with two carve-outs already priced in: X9 (dream run) and design work
  that gates a Now item.
- 24/7 flywheel continuity (06-20 stack item 7): after N1-N4, by its own original
  admission ("after 1-6 are stable").

## Changes from the previous roadmap (2026-08-18, decision-ready)

1. Canonical file. Was: ROADMAP-FLYWHEEL-2026-06-10.md in engram-research, marked
   canonical, 267-bead baseline, untouched since June. Now: this file at dear-agent
   root, per the never-executed proposal in PR #322 Part 3; supersedence banner owed
   to the flywheel roadmap (follow-up, engram-research). Named per the Living
   Documentation Policy: ROADMAP.* is a forbidden temporal artifact here, enforced
   by cmd/routing-guard. Evidence: PR #322 verdict "stale, partial"; active queue
   is 968, 3.6x the baseline.
2. Top theme. Was: flywheel wiring first (traces to retros to evals). Now:
   continuous-execution friction removal (N1-N4), flywheel increments gated behind
   it (X8, X9). Evidence: week of 08-11..18: 12/32 spawns wedged, 56 OAuth family
   deaths, two queue-wide CI outages, relay dead 24 days. Friction, not idea supply,
   was binding (friction-priorities doc; PR #327).
3. Trace-mining epics (ce-asrxq, ce-qrwt3, ce-nsuo8). Were: fresh Fable designs
   (08-14), implicitly next up. Now: X8 gated on sensors, schema, and truthful
   session data; ce-nsuo8 parked. Evidence: signals.db empty (ce-qf0f); dep graph
   does not encode stated prerequisites; Kaggle deadline passed.
4. OAuth. Was: item 6 of the 06-20 stack, estimated one day. Now: half of N2, with
   deploy receipts and family-death alarms, plus the ce-77ip single-writer epic as
   the L follow-on. Evidence: 414 death markers; recurrence across three retros.
5. P0 bankruptcy. Was: 07-26 order for one triage pass, cap 12, never executed.
   Now: N5 with harder caps (5 P0 / 15 P1) from PR #322. Evidence: P0s doubled
   (37 to 72) since the order was written.
6. Host safety floor (07-26 CP-1). Was: top critical path. Now: absorbed into N4
   as the liveness-and-heartbeat class, since the class, not the instance, recurred.
   Evidence: disk retro shows the 07-18 fix missed the already-dead sandbox GC.
7. Truth reconciliation (07-26 CP-2). Was: CP-2. Now: split into N3 (pipeline
   truth) and X6 (claim truth), in that order. Evidence: the 08-11 twin retros show
   they are distinct failures that compound.
8. Factorio shortlist (PR #321). Was: standalone 8-item research shortlist. Now:
   folded in as X7 and X10; items overlapping friction work (breakers, hysteresis,
   admission) merged into N1/N4 rather than duplicated. Evidence: analogy matrix
   maps 1:1 onto observed failures (cap saturation, byproduct branch leak).
9. Escalation wiring (07-26 CP-3). Was: CP-3. Now: inside N3, since the relay
   dead-letter retro showed escalation and results share one broken transport.
10. Dreaming plan (08-17). Was: new research, unranked. Now: X9 admitted ahead of
    the trace miner because it is days of work with zero infrastructure and fixes a
    live defect (over-limit MEMORY.md). Evidence: dreaming plan's own priority order.
11. Review-comment mining (new, X2). Was: absent; review rounds were treated purely
    as latency to bound (old X1). Now: first-class self-improvement mechanism, with
    X1 reframed as the backstop bound. Automated review and CI are a backstop, not
    the agent's primary way to learn; each recurring review finding becomes a
    just-in-time instruction at the right placement level, never top-level AGENTS.md
    unless universal. North-star metric: automated-review catches trending toward
    zero. Evidence: operator reframe 2026-08-18 (this revision); the input corpus
    is already banked (#1096, #1243), the cheapest dreaming-pattern loop to start.

## Pending inputs to reconcile

- The HZLPhPbw3fM research named in the refinement brief: not found anywhere in
  engram-research (files, commit messages, all refs) as of 2026-08-18. Reconcile at
  next review if it lands, or correct the reference.
- engram-research PR #322 (strategic state) is still open: merge it as the audit
  trail behind this file.
- Local golden clones diverged from origin (7 repos): N4 covers the alarm.

## Reconciliation exceptions

None yet. First weekly reconciliation due 2026-08-25.
