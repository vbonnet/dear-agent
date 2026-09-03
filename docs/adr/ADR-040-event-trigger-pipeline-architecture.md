# ADR-040: Event/trigger to pipeline to transform to emit architecture

Status: Proposed (2026-09-02)

## Context

dear-agent has built pieces of the same shape at least six times, independently: VROOM
defines decision topic constants and an `Emitter` type
(`pkg/vroom/vroom/emitter.go`, `topics.go`) with zero production callers; Wayfinder
emits phase events (`wayfinder/internal/analytics/session_tracker.go`); the trigger
registry (`pkg/trigger`) matches events to engrams and is genuinely wired to the
in-process event bus (`pkg/trigger/subscriber.go` subscribes to it), but only on three
hardcoded topics, with exact-match lookup and a single hardcoded action; the
absence-alarm (`pkg/absencealarm`, unmerged) models expected-event pulses as a host
watchdog; the GitHub webhook receiver (`cmd/agm-webhook-receiver`) is explicitly spike
scaffolding, not wired into a build target; and `pkg/workflow` (ADR-010) runs durable,
audited, HITL-capable workflows that nothing currently starts from an event.

None of these converge, and the concrete, felt cost is the PR-queue throughput problem.
`internal/mergeloop/driver.go:164-168` correctly detects when open PRs exceed its cap
and skips the tick with an audited `backpressure` action, but that signal reaches only
mergeloop's own audit log. Nothing else in the system, an operator, another pipeline, a
capacity decision, can react to it. The system has the sensor; it has no way to carry
the signal anywhere else.

A prior draft of the design this ADR is based on proposed a new `pkg/eventbus`-based
transport for all of this and a new `pkg/pipeline` runtime to host workflows.
Adversarial review found that `pkg/eventbus.LocalBus` documents itself as "an
in-process event bus implementation" (`pkg/eventbus/local_bus.go:17`) and does not
cross a process boundary; every example flow this design is meant to serve originates
in a different process than the one that should react to it (mergeloop and
absence-alarm are separate launchd one-shots, VROOM supervisors are separate harness
sessions). The repo already has a cross-process durable transport, `agm-bus`
(`agm/internal/bus`, `agm/cmd/agm-bus`), explicitly designed to carry
"side-channel observability events from infrastructure code... that originate outside
any running Claude session" (`agm/internal/bus/emitter.go`). Review also found
`pkg/workflow` is a strict superset of what a new `pkg/pipeline` package would need to
provide (durable state, per-transition audit, HITL). This ADR is revised accordingly.

Full design and diagrams:
[docs/architecture/feedback-loop-pipelines.md](../architecture/feedback-loop-pipelines.md).

## Decision

Adopt a shared event and trigger vocabulary across dear-agent, built by extending
existing infrastructure, not replacing it, and split into what this ADR decides now
versus what it explicitly defers to its own design review.

Decided now:

1. **`agm-bus` is the cross-process transport** for any event that needs to reach a
   different process than the one that emitted it. No new cross-process bus gets
   built. `pkg/eventbus` stays as the in-process fan-out layer inside one running
   process; it is not extended to cross process boundaries.
2. **`pkg/trigger` is extended, not replaced**: its subscriber's topic list becomes
   derived from the registry's registered event types instead of hardcoded, and its
   matcher gains wildcard support aligned with `pkg/eventbus`'s existing pattern
   matcher, so a registered trigger for a new event type is actually reachable.
3. **VROOM, Wayfinder, Beads, and mergeloop keep their current ownership and
   mechanics unchanged.** This ADR adds emitted events alongside their existing
   behavior; it does not change who owns roadmap state, dispatch, or merge mechanics.
   ADR-002's "Beads are the roadmap" invariant is explicitly preserved.
4. **The Wayfinder-to-trigger-registry topic-name mismatch gets fixed as its own
   small change**: Wayfinder emits `wayfinder.phase.started`, while the documented
   engram `TriggerSpec.On` value and the registry's exact-match lookup expect
   `phase.started`. Any engram declared against the bare name is unreachable today.
   This is real, small, and independently shippable ahead of everything else in this
   ADR.

Explicitly deferred to a follow-up design review, not decided here:

5. **An event-triggered `pkg/workflow`**, so a workflow run can start from a trigger
   match instead of only a CLI invocation, replacing the earlier plan to build a new
   `pkg/pipeline` package from scratch.
6. **A unified event envelope**, extending `pkg/eventbus.Event` with provenance and
   related-ID fields, and a channel-naming convention that does not silently drop
   proposed event names (`bead.ready`, `paper.posted`, and the rest) into undurable
   channels the way `ChannelFromType`'s current first-segment rule does.
7. **A generic backpressure policy package**, generalizing mergeloop's proven
   cap-and-skip logic so any workflow run can declare a capacity limit the same way.
8. **A merge decision for benchmark-driven changes** (the "arXiv paper to competing
   PRs to benchmark to merge decision" flow). This is a governance decision, not a
   plumbing one, and needs the same scrutiny ADR-030 got, checked against ADR-032's
   must-reach-human taxonomy and ADR-031's exception-path discipline. It is not
   pre-authorized by this ADR; this ADR only names where it would plug in.

## Alternatives considered

- **Build a new in-process-to-cross-process event bus from scratch.** Rejected:
  `agm-bus` already solves the cross-process case (durable per-session queue, ACL,
  already used for infrastructure-origin events), and `pkg/eventbus` already solves
  the in-process case. A new bus would be a seventh independent implementation of a
  shape this ADR exists to stop reproducing.
- **Build a new `pkg/pipeline` runtime.** Rejected: `pkg/workflow` (ADR-010) already
  provides durable state, per-transition audit, and first-class HITL, everything a new
  runtime would need to reinvent. The gap is narrower than a new package: `pkg/workflow`
  is not currently startable from an event.
- **Make beads readiness push-based instead of polled.** Rejected for now: ADR-002 is
  explicit that VROOM does not own roadmap projections, and beads' Dolt-backed store is
  a separate system this repo consumes through the `bd` CLI, not one it controls the
  internals of. Any bead-state event this design adds is a differ watching `bd ready`'s
  output change, which is itself a poll with an extra step; that is named honestly in
  the design doc rather than claimed as push-based.
- **Make VROOM subscribe to events instead of polling.** Rejected as a direct
  substitution: VROOM's supervisors are LLM harness sessions on independent tick
  timers (`/loop` prompts), not Go processes running an event loop. "Subscribe" is not
  a small change here; delivering a matched event into a supervisor's next tick prompt
  as pre-fetched context is a smaller, testable step, listed as a proposal to measure,
  not a settled design.
- **Fold the absence-alarm into the trigger router as just another subscriber.**
  Rejected: absence-alarm's own SPEC names "the watcher lives inside the thing being
  watched" as the exact anti-pattern it exists to avoid; making it depend on the same
  transport it is meant to watch for silence would reintroduce that failure mode. It
  should emit onto `agm-bus` for visibility without depending on that transport to
  function.
- **Ship the merge decision as part of this ADR.** Rejected: adversarial review found
  concrete, non-cosmetic gaps in an earlier draft of that decision (the gate's own
  safety constants were not excluded from its own auto-merge class, no requirement
  that the benchmark be independent of the change it measures, no composition with
  ADR-031/ADR-032, no single-winner rule among competing candidates). Each of those is
  a real path by which an agent's own change could ship without a human catching it,
  and none of them is a plumbing question this ADR's other decisions can settle as a
  side effect.

## Consequences

- Mergeloop's existing backpressure signal becomes visible system-wide instead of
  buried in its own audit log; the PR-queue throughput problem becomes observable at
  the point of contention.
- A real, currently-shipping defect (the Wayfinder-to-trigger topic-name mismatch)
  gets fixed as a direct consequence of writing this design down, independent of
  everything else in it.
- The cross-process transport question, previously unasked, is answered: `agm-bus`,
  not a new bus and not `pkg/eventbus` stretched past its documented scope.
- New surface area is smaller than an earlier draft proposed: extending `pkg/workflow`
  and `pkg/trigger` instead of building a new package each.
- Event schema versioning and delivery-ordering guarantees remain open questions,
  deferred to the design review for items 5 through 7 above, not resolved here.
- The merge decision, once designed, needs the same scrutiny ADR-030 got; this ADR
  does not pre-authorize it, only names where it would plug in.

## References

- [docs/architecture/feedback-loop-pipelines.md](../architecture/feedback-loop-pipelines.md), full design, event taxonomy, LTN analogy, example flows
- [docs/architecture/feedback-loop-diagrams.md](../architecture/feedback-loop-diagrams.md), C4 Context/Container diagrams and flow sequence diagrams
- [ADR-002](ADR-002-vroom-execution-architecture.md), VROOM topology, "Beads are the roadmap"
- [ADR-010](ADR-010-workflow-engine-architecture.md), existing durable workflow engine
- [ADR-029](ADR-029-ralph-wiggum-merge-loop.md), mergeloop's host-tick design
- [ADR-030](ADR-030-dependabot-auto-merge.md), existing narrow, mechanical auto-merge precedent
- [ADR-031](ADR-031-agent-escalation-path.md), [ADR-032](ADR-032-escalate-to-supervisor.md), exception-path discipline and must-reach-human taxonomy
