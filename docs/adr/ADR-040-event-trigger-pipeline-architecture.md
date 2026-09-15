# ADR-040: Event/trigger to pipeline to transform to emit architecture

Status: Proposed (2026-09-02)

## Context

dear-agent has built pieces of the same shape at least six times, independently: VROOM
defines decision topic constants and an `Emitter` type
(`pkg/vroom/vroom/emitter.go`, `topics.go`) with zero production callers; Wayfinder
emits phase events into a fresh, unsubscribed in-process bus on every CLI invocation
(`wayfinder/cmd/wayfinder-session/internal/tracker/tracker.go`); the trigger registry
(`pkg/trigger`) matches events to engrams and has a subscriber implemented and tested
(`pkg/trigger/subscriber.go`), but with zero production callers outside a CLI that
evaluates one synthetic event at a time; the absence-alarm (`pkg/absencealarm`,
committed but unmerged) models expected-event pulses as a host watchdog; the GitHub
webhook receiver (`cmd/agm-webhook-receiver`) is compiled and tested but has no
deploy path; and `pkg/workflow` (ADR-010) runs durable, audited, HITL-capable
workflows, invokable today from the CLI and MCP, but not from an event.

None of these converge, and the concrete, felt cost is the PR-queue throughput problem.
`internal/mergeloop/driver.go:164-168` correctly detects when open PRs exceed its cap
and skips the tick with an audited `backpressure` action, but that signal reaches only
mergeloop's own audit log. Nothing else in the system, an operator, another pipeline, a
capacity decision, can react to it.

Two further constraints, found during design review, narrow what this ADR can decide
today. First, `pkg/eventbus.LocalBus` documents itself as "an in-process event bus
implementation" (`pkg/eventbus/local_bus.go:17`) and does not cross a process
boundary; every trigger source this design serves (mergeloop, absence-alarm, VROOM
supervisors) runs as a separate process from the one that should react to its events.
`agm-bus` (`agm/internal/bus`, `agm/cmd/agm-bus`) is the repo's durable cross-process
broker, but its wire protocol has no publish/subscribe frame type today: all nine frame
types are addressed to a single session id, and the `Emitter.EmitEvent` convenience
that looks like a broadcast sends a frame the server's own validation rejects. Second,
`pkg/trigger` lives outside `agm/` in the module's import graph
(`github.com/vbonnet/dear-agent/pkg/trigger`), so it cannot import `agm/internal/bus`
directly under Go's internal-package visibility rule. The repo already has the answer
to that second constraint: `agm/workflowbus/bridge.go` connects `pkg/workflow`, also
outside `agm/`, to the same broker, and its own doc comment names the exact reason it
lives under `agm/`.

Full design and diagrams:
[docs/architecture/feedback-loop-pipelines.md](../architecture/feedback-loop-pipelines.md).

## Decision

Adopt a shared event and trigger vocabulary across dear-agent, built by extending
existing infrastructure, not replacing it. This ADR decides the direction; the actual
protocol and package design for items 4 through 8 below still needs its own review
before implementation beads get filed against them, exactly as the design document
scopes it. Item numbers below match the design document's numbering exactly.

Decided now, on direction:

0. **Fix the Wayfinder to trigger-registry topic-name drift** as a small, independent
   first step: pick one naming convention and correct every variant to match. This
   alone does not make a trigger fire end to end; see item 6.
1. **Extend `pkg/trigger`'s subscriber to derive its subscription set from the
   registry**, instead of the hardcoded topics in `pkg/trigger/subscriber.go` today.
2. **Add wildcard matching to `pkg/trigger/registry.go`**, aligned with
   `pkg/eventbus`'s existing pattern matcher, so the two components agree on what a
   subscription pattern means.
3. **`agm-bus` is the cross-process transport**, once it has a publish/subscribe
   primitive (item 5). No new cross-process bus gets built. `pkg/eventbus` stays as the
   in-process fan-out layer inside one running process; it is not extended to cross
   process boundaries.
4. **VROOM, Wayfinder, Beads, and mergeloop keep their current ownership and
   mechanics unchanged.** This ADR adds emitted events alongside their existing
   behavior; it does not change who owns roadmap state, dispatch, or merge mechanics.
   ADR-002's "Beads are the roadmap" invariant is explicitly preserved.
5. **A bridge package under `agm/`, not a new package under `pkg/`, is the pipeline
   runtime's production home**, following `agm/workflowbus/bridge.go`'s precedent, and
   `pkg/workflow` (ADR-010), not a new `pkg/pipeline` package, is the durable execution
   engine it starts runs on.

Deferred to a follow-up design review, not decided in detail here:

6. **The unified event envelope** (design doc item 4) and **the `agm-bus`
   publish/subscribe wire-protocol addition** (design doc item 5): these are protocol
   and schema designs, not wiring exercises, and need their own review.
7. **The `agm/`-rooted bridge package's exact contract** (design doc item 6): the
   event-to-workflow trigger-action model (input mapping, authorization, idempotency,
   failure and retry ownership) is not yet an interface, and needs to be one before
   this is implementable.
8. **A generic backpressure policy package** (design doc item 7): mergeloop's `cap(N)`
   is proven; `timeout(D)` and `staleness(D)` are new designs this ADR does not treat
   as proven.
9. **A merge decision for benchmark-driven changes** (design doc item 8, the "arXiv
   paper to competing beads to benchmark to merge decision" flow). This is a
   governance decision, not a plumbing one, and needs the same scrutiny ADR-030 got,
   checked against ADR-032's must-reach-human taxonomy and ADR-031's exception-path
   discipline. It is not pre-authorized by this ADR; this ADR only names where it would
   plug in.

## Alternatives considered

- **Build a new cross-process event bus from scratch, instead of extending
  `agm-bus`.** Rejected: `agm-bus` already provides the durable per-session queue and
  ACL a new bus would need to reinvent; the missing piece is one wire-protocol
  addition (item 6), not a new transport.
- **Have `pkg/trigger` import `agm/internal/bus` directly.** Rejected: Go's
  internal-package visibility rule forbids it, since `pkg/trigger` is outside the
  `agm/` prefix. `agm/workflowbus/bridge.go` already establishes the correct pattern
  for this exact constraint, and this ADR follows it rather than proposing something
  the compiler would reject.
- **Build a new `pkg/pipeline` runtime.** Rejected: `pkg/workflow` (ADR-010) already
  provides durable state, per-transition audit, and first-class HITL, and is already
  invokable from more than the CLI (an MCP tool exists). The gap is narrower than a
  new package: `pkg/workflow` is not currently startable from an event.
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
  should emit onto `agm-bus` for visibility once that transport can carry it, not
  depend on it to function.
- **Route Flow B's candidate PRs directly from a worker spawn, bypassing beads.**
  Rejected: the research-pipeline skill's actual Stage 4 output is decomposition into
  sized beads, not a worker spawn, and routing candidates through beads means VROOM's
  existing dispatch capacity governs how many candidates are in flight at once, instead
  of a second, duplicate admission check.
- **Ship the merge decision as part of this ADR.** Rejected: design review found
  concrete, non-cosmetic gaps in an earlier version of that decision (the gate's own
  safety constants were not excluded from its own auto-merge class, no requirement
  that the benchmark be independent of the change it measures, no composition with
  ADR-031/ADR-032, no single-winner rule among competing candidates). Each of those is
  a real path by which an agent's own change could ship without a human catching it,
  and none of them is a plumbing question this ADR's other decisions can settle as a
  side effect.

## Consequences

- Mergeloop's existing backpressure signal becomes visible system-wide instead of
  buried in its own audit log, once `agm-bus` gains a publish/subscribe primitive; the
  PR-queue throughput problem becomes observable at the point of contention.
- Fixing the Wayfinder to trigger-registry topic-name drift is real, independently
  useful work, but does not by itself make a trigger fire; production composition
  (item 5) and the transport addition (item 6) are separate prerequisites.
- The cross-process transport question, previously unasked, is answered in direction:
  `agm-bus`, not a new bus, once it has the protocol addition item 6 scopes.
- New surface area is a bridge package under `agm/` and a protocol addition to
  `agm/internal/bus`, not a new top-level `pkg/pipeline` package.
- Event schema versioning and delivery-ordering guarantees remain open questions,
  deferred to the design review for items 6 through 8, not resolved here.
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
