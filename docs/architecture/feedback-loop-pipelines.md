# Feedback-Loop Pipelines: Event/Trigger to Pipeline to Transform to Emit

<!-- Last audited at: 2026-09-02 -->

Status: Proposed. This document is a design proposal, not a description of a shipped
system. Sections are marked EXISTING (running in a production binary today, cited) or
NEW (not built yet). An EXISTING label means the wiring runs, not merely that the code
exists somewhere in the tree; several claims in an earlier draft of this document
conflated the two, and this revision corrects each one it found.

## The idea in one sentence

An event or trigger starts a pipeline; the pipeline transforms it and emits a new
event; that event can start the next pipeline. Chained, this is a feedback loop: the
system's own output becomes another part of the system's input, and the loop is how
the system observes and corrects itself over time.

We already build individual pieces of this shape, more than once, independently (see
[§ What already exists](#what-already-exists) below). What is missing is a shared
vocabulary: one event envelope, one place to declare "on event X, start pipeline Y,"
and one place to express "do not start Y if the network does not have capacity right
now." This document proposes that shared vocabulary and shows how it would connect
pieces that already exist (VROOM, Wayfinder, Beads, mergeloop, the absence-alarm)
without replacing any of them.

## Diagrams

[feedback-loop-diagrams.md](feedback-loop-diagrams.md) has the System Context and
Container diagrams (C4, Mermaid) and two sequence diagrams for the example flows below.
Read this document first; the diagrams are the compressed version.

## What makes a loop a feedback loop

Not every loop is a feedback loop. A `for` loop that reprocesses the same queue is not
one unless what comes out changes what goes back in. Every flow in this document is
judged by one test: does the emitted event carry information the next stage did not
already have? If not, it is polling with an extra step, and this document says so
plainly rather than dressing up a poll as an event. Flow A's final step, discussed
below, is an example this document deliberately does not add an event to, because it
fails that test.

## The model

```
  EVENT / TRIGGER  ->  PIPELINE  ->  TRANSFORM(S)  ->  EMIT NEW EVENT  ->  (loop)
```

- **Event or trigger**: something happened, worth acting on. A ticket was filed. A
  cron tick fired. A paper was posted. An agent raised a process problem. A bead
  became ready. Not every event is externally caused; an agent proposing a process
  improvement is a trigger it raises about itself.
- **Pipeline**: a named, addressable unit of work that a trigger router matches an
  event against and runs. A pipeline is not necessarily code; the research-pipeline
  skill is a pipeline whose stages are run by whichever agent or model the routing
  rule names.
- **Transform**: what the pipeline actually does to the event's payload: triage,
  classify, plan, decompose, benchmark, merge. A pipeline can have one transform stage
  or several in sequence, as the research-pipeline's five stages do.
- **Emit**: the pipeline's output is itself a new event on the bus, not just a side
  effect. This is the part that is easy to skip and is exactly what makes the chain a
  feedback loop instead of a one-off script. A pipeline that writes a file and stops
  is a transform. A pipeline that writes a file and emits `plan.approved` so the next
  pipeline can pick it up is a feedback loop.

## Event taxonomy

| Trigger class | Examples | Today | Proposed |
|---|---|---|---|
| **External ingress** | GitHub webhook (PR opened, check completed), arXiv/RSS new paper | `cmd/agm-webhook-receiver` writes a bespoke JSONL file, and its own header marks it spike scaffolding not wired into a build target (EXISTING as a spike) | Generalize: any external source lands as a typed event on the trigger router, not a bespoke file per source |
| **Scheduled tick** | launchd cron: mergeloop, absence-alarm, disk-watchdog, sandbox-gc | Each tool polls on its own interval, independently (EXISTING) | Ticks stay ticks; they are triggers too. But a tick's findings (a PR queue backed up) become an emitted event other pipelines can react to, not just a log line |
| **State transition** | Bead becomes ready, blocked, or closed; PR merged; escalation resolved | Polled, not emitted (confirmed gap, see [§ What already exists](#what-already-exists)) | State transitions become emitted events (`bead.ready`, `pr.merged`, `escalation.resolved`) alongside the existing state write, not instead of it |
| **Agent-raised** | Worker raises a process problem, an agent proposes a process improvement | `agm escalate` (EXISTING, for blocking questions and decisions; routed through `pkg/vroom/escalation`'s classifier, ADR-031, ADR-032) | Extend to non-blocking "I noticed X" events that do not require a human answer to proceed. Today escalation assumes something is blocked; a proposal is not |
| **Absence** | Expected event did not arrive: a merge, a heartbeat, a sweep | `pkg/absencealarm` (a periodic host state probe: file mtime, launchd job presence, command exit; not itself event-driven, see [§ Absence-alarm](#absence-alarm-is-the-inverse-signal-not-an-event-consumer)) | Wire its alarm and recovery transitions onto `agm-bus`, so "the mergeloop went dark" is the same kind of signal as "a PR merged," searchable in the same place |
| **Pipeline output** | A pipeline's own emitted event | Ad hoc per pipeline (Wayfinder's phase events are the clearest real example) | Standardize the envelope so any pipeline's output can trigger any other pipeline, not only the one it was written for |

## The Factorio Logistics Train Network analogy

Valentin's reference point, made explicit because it is a genuinely good structural
match, not just a fun comparison, and because getting the mechanics right matters more
than the analogy being memorable.

In Factorio's LTN mod, stations declare what they have (provide) or need (request) as
item and quantity thresholds. The mod does not move anything itself; it watches the
declared requests and provides, and when a request can be matched to a provider with
enough stock and a free train, it dispatches a train to carry that specific cargo
between those two specific stations. A request that cannot be matched, because no
provider clears its threshold or no train is free, does not fail loudly on its own. It
just does not get served, and the LTN community built a companion mod (LTN Manager)
specifically because that starvation is otherwise invisible without extra tooling: you
have to go look at the request queue to notice a station has been waiting.

| LTN concept | This architecture |
|---|---|
| A station's request ("I need 200 iron plates") | An event a pipeline is waiting on, a trigger subscription |
| A station's provide ("I have 500 copper ready") | A pipeline's emitted event, something now available for the next stage |
| The LTN dispatcher matching requests to provides | The trigger router matching emitted events to subscribed pipelines |
| A train carrying cargo between two stations | A worker, an agent session executing one pipeline run |
| Per-station train limit | Concurrency limit on how many pipeline runs can be in flight at once |
| Provide/request stack threshold (minimum cargo per dispatch) | Batching granularity: how large a request has to get before it is worth dispatching a run for, the "fewer, larger requests" relief lever discussed below |
| Network starvation, invisible unless you instrument it | Our actual, lived problem: the PR queue backing up faster than review capacity clears it, invisible in mergeloop's own audit log today (see [§ Backpressure](#backpressure-and-throughput)) |
| A station that stops requesting or providing and nobody notices | What the absence-alarm exists to catch |

The analogy earns its keep on the point above: LTN starvation is not naturally
visible, it becomes visible when someone instruments the queue at the point of
contention. Our mergeloop backpressure check (`internal/mergeloop/driver.go:164-168`,
verified: `if len(prs) > maxOpen` audits a `backpressure` action and skips the tick) is
that instrument, correctly built, in exactly the right place. The gap is that its
signal dies in mergeloop's own audit log instead of reaching anyone who could relieve
it. Making that signal visible where the rest of the system can see it is what this
proposal is actually for.

## Example Flow A: simple, ticket to assigned worker

See the sequence diagram in [feedback-loop-diagrams.md, Flow
A](feedback-loop-diagrams.md#3-flow-a-simple-ticket-to-assigned-worker-as-a-sequence).

1. A ticket is filed on GitHub, or by an agent, or by a human in chat.
2. `ticket.created` reaches the trigger router.
3. The router matches it to a triage run: classify severity, assign a priority,
   create a bead with acceptance criteria (`bd create`).
4. Beads' own dependency graph resolves the bead to "ready" when its dependencies, if
   any, close. This step stays poll-based: VROOM's Meta-Orchestrator reads `bd ready`
   on its normal 180-second tick (ADR-002's "Beads are the roadmap" invariant is
   unchanged by this proposal; VROOM is not asked to abandon polling for the roadmap
   it owns, and beads' own Dolt-backed store is external to this repo, not something
   dear-agent's event bus can emit from directly).
5. VROOM dispatches a worker through its existing AGM path.
6. The worker opens a PR.

This flow deliberately stops there. An earlier draft added a `worker.pr_opened` event
at this point; it does not survive this document's own test. Mergeloop's existing
10-minute tick discovers the new PR on its own next pass regardless, so emitting an
event here would carry no information mergeloop's tick does not already get, and
ADR-029 already chose a host-tick design over webhooks for mergeloop for reasons this
proposal is not trying to relitigate. Flow A is included as the baseline case, so the
advanced flow below has something to be advanced relative to; it is not itself a
strong argument for anything new.

## Example Flow B: advanced, arXiv paper to a merge decision

See the sequence diagram in [feedback-loop-diagrams.md, Flow
B](feedback-loop-diagrams.md#4-flow-b-advanced-arxiv-paper-to-auto-merge-or-human-review).

1. An RSS or arXiv feed poster emits `paper.posted`.
2. The trigger router matches it to the research-pipeline skill (EXISTING,
   `research-pipeline/skills/research-pipeline/SKILL.md`), started as a workflow run:
   Stage 2 does goal-oriented research on how the paper's idea might apply to
   dear-agent; Stage 3 hands off to an independent model that adversarially
   fact-checks the research and writes a codebase-grounded plan.
3. The plan hits the skill's existing human review gate. This is not new; it is the
   skill's own Stage 3 to Stage 4 boundary, and the approval receipt stays bound to
   the exact plan revision (commit SHA or content digest), as the skill already
   requires.
4. Stage 4 runs next, unchanged from the skill: a third independent model
   adversarially reviews the approved plan before any bead exists. If it finds a
   blocking defect, the plan goes back to Stage 3's author to fix, then Stage 4
   re-reviews, with a fresh approval receipt bound to the corrected revision. An
   earlier draft of this flow skipped straight from the human gate to spawning
   workers, silently dropping this review; this revision keeps it, because it is the
   mechanism that makes the research-pipeline's output trustworthy enough to fan out
   into competing PRs in the first place.
5. Once Stage 4 ships unconditionally, `plan.approved` triggers N competing
   experimental workers, one per candidate approach.
6. Each candidate PR triggers `benchmark.requested`. A backpressure policy, generalized
   from mergeloop's cap, admits, queues, or rejects the run based on current
   concurrent-benchmark count: the LTN "is a train free" check.
7. All N candidates are measured against one shared, independently authored benchmark,
   not one each candidate writes for itself. The research-pipeline skill's own rule for
   Stage 3 and Stage 4 review, "never let one model mark its own homework," applies
   here too: a benchmark authored by the same lineage that wrote the change it is
   measuring is not independent.
8. The benchmark pipeline emits `benchmark.completed(pr, delta)`. The delta is the
   payload that makes this a feedback loop and not a poll: the next stage does not have
   to go re-measure anything.
9. A merge decision evaluates the delta against the change's risk classification. This
   document sketches, but does not ship, that decision; see the next section for why.

## Auto-merge illustration, not a decision

An earlier draft of this document included a five-row decision table deciding when a
benchmark-confirmed change auto-merges without a human, and ADR-040 initially adopted
it as a decision alongside the plumbing changes. Adversarial review found real,
concrete gaps in that table, not stylistic ones:

- The table's own thresholds and safety constants were not excluded from its
  "auto-merge eligible" class. `internal/mergeloop/driver.go`'s `maxOpen` cap,
  `cmd/vroom-dispatch`'s spawn-interval circuit breaker, and the absence-alarm's clock
  skew tolerance are all single-package constants a benchmark could show "improved"
  while quietly disabling the safety property they exist to enforce.
- Nothing required the benchmark itself to be independent of the change under test
  (see point 7 above); the table as drafted let a change and its own measurement come
  from the same lineage.
- "Same shape as an already-accepted auto-merge class" was left to agent judgment with
  no expiry or re-audit, unlike ADR-030's actual precedent, which is a mechanical
  classifier (bot identity, semver bump type, immutable head binding), not a judgment
  call. Comparing this table to ADR-030 as though they offered the same kind of
  guarantee was not accurate, and this revision does not repeat that comparison.
- The table's "foundational change" test was a path check (did the diff touch
  `SPEC.md`, `CONTEXT.md`, an ADR, or a public interface). A change that contradicts an
  existing SPEC requirement's behavior without editing the SPEC file would pass that
  check, which is exactly the shape of a change worth catching, not waving through.
- Nothing selected a single winner among the N competing candidate PRs, so more than
  one "improving" candidate could auto-merge against the same idea.
- The table did not compose with `pkg/vroom/escalation`'s existing must-reach-human
  taxonomy (ADR-032: product, pricing, security, destructive, legal, spend, people, and
  external-communication decisions) or with ADR-031's rule that repeated use of an
  exception path is itself a defect signal.

None of these are cosmetic; each one is a real path by which an agent's own change
could ship without a human catching it. This document is not the place to resolve them:
the merge decision needs the same scrutiny ADR-030 got, worked out on its own, against
ADR-032's taxonomy and ADR-030's mechanical-classifier bar, not adopted as a side
effect of the plumbing proposed here. Flow B's sequence diagram shows where the
decision plugs in and stops there; every path out of it in this document's own diagram
routes to human review.

## Backpressure and throughput

Two things need to be true for the LTN analogy to help operationally, not just
narratively.

First, a queue depth must be visible at the point of contention, not buried in one
tool's private log. Today, mergeloop's cap-exceeded check
(`internal/mergeloop/driver.go:164-168`) is exactly the right signal, generated in
exactly the right place, but it only reaches mergeloop's own audit log; nothing
downstream can react to it. Proposal: the same check emits `pr.backpressure(open, cap)`
over `agm-bus`. That single change makes the PR-queue throughput problem observable in
the same place as everything else, instead of something a human has to notice by
reading mergeloop's audit file.

Second, backpressure must be a declarable policy, not a per-tool special case.
Mergeloop has one (`--cap`, default 50). Nothing else does. The proposed backpressure
policy (`pkg/pipeline/backpressure`, new) expresses `cap(N)`, `timeout(D)`, and
`staleness(D)` once, extracted from mergeloop's already-proven implementation rather
than designed from scratch, so mergeloop becomes the first consumer of the generalized
version instead of a second implementation existing alongside it.

Relief, in LTN terms, is either adding capacity (more trains: more review bandwidth,
more worker slots) or reducing demand (fewer, larger requests: batch tickets, raise the
bar for what auto-triggers a pipeline). This document does not propose which lever to
pull in general; it proposes that the queue depth be visible enough that pulling one is
a decision, not a discovery three weeks later.

## Absence-alarm is the inverse signal, not an event consumer

`pkg/absencealarm` and `cmd/absence-alarm` (branch `stack/absence-alarm-cli`, stacked
on `stack/absence-alarm-domain`, not yet in `main`) is worth naming explicitly because
it inverts everything above: instead of "event X arrived, run a pipeline," it is
"event X was expected and did not arrive, alarm." Its `Pulse` primitive is a
periodically probed host state check (a file's modification time, whether a launchd
label appears in the job listing, whether a command exits zero), each with a maximum
silence window. It is a poll, not an event consumer; this document is explicit about
that rather than dressing it up, per its own test above.

This proposal does not fold absence-alarm into the trigger router's model. It stays a
purpose-built host-level watchdog, deliberately independent of the mesh, by design: its
own SPEC names "the watcher lives inside the thing being watched" as the exact
anti-pattern it exists to avoid, and wiring it as just another subscriber would
reintroduce that failure mode. What this proposal does ask for is narrower:
absence-alarm's alarm and recovery transitions should also land on `agm-bus`
(`pulse.absent`, `pulse.recovered`) so an operator or another pipeline can see "the
mergeloop went dark" as the same kind of signal as "a PR merged," without
absence-alarm depending on that transport being alive to detect that the transport
itself might be the thing that is dark.

## What already exists

This is the load-bearing distinction the proposals below depend on. An earlier draft of
this table mislabeled several rows as EXISTING when only the code existed, not the
wiring; this revision corrects each one and cites the verification.

| Piece | Status | Where |
|---|---|---|
| In-process event bus (four channels, Subscribe/Emit/AddSink, typed `Event`) | **EXISTS**, in-process only, `pkg/eventbus/local_bus.go:17` documents itself as "an in-process event bus implementation" | `pkg/eventbus/bus.go`, `pkg/eventbus/local_bus.go` |
| Cross-process durable transport (unix socket, per-session offline queue, ACL) | **EXISTS**, built for session and infrastructure-event routing, not yet used for pipeline triggers | `agm/internal/bus`, `agm/cmd/agm-bus`, especially `agm/internal/bus/emitter.go` |
| Durable workflow engine (YAML dependency-ordered nodes, SQLite state, per-transition audit, HITL) | **EXISTS**, general-purpose, not event-triggered | `pkg/workflow` (ADR-010) |
| Trigger registry, event type to engram dispatch, wired to the in-process bus | **EXISTS and runs**: `pkg/trigger/subscriber.go` subscribes to three hardcoded topics on `pkg/eventbus`. Exact-match only (`pkg/trigger/registry.go`'s lookup is a plain map keyed on the literal event type string, no wildcard), single hardcoded action (engram context injection) | `pkg/trigger/registry.go`, `subscriber.go`, `matcher.go` |
| VROOM decision topic constants and an `Emitter` type | **Defined, not wired**: zero non-test call sites for `EmitDispatched`/`EmitEscalated`/`EmitEvaluated`/`EmitGated`/`EmitHandedOff` anywhere in the repo; the decision trail (below) is what actually runs | `pkg/vroom/vroom/emitter.go`, `topics.go` |
| Append-only decision log (JSONL) | **EXISTS**, fragmented across separate implementations (decision trail, escalation log, webhook receiver's own file) | `pkg/vroom/decisiontrail/trail.go`, `pkg/vroom/escalation` |
| Wayfinder phase events | **EXISTS and runs**, but under the namespaced topics `wayfinder.phase.started` and `wayfinder.phase.completed`, not the bare `phase.started`/`phase.completed` that `pkg/engram/trigger.go`'s documented `TriggerSpec.On` values and the registry's exact-match lookup expect. Any engram declaring `on: phase.started` today is unreachable: the event that would match it is never emitted under that exact string | `wayfinder/internal/analytics/session_tracker.go:68,101`, `pkg/engram/trigger.go`, `pkg/trigger/registry.go` |
| GitHub webhook ingress | **EXISTS as spike scaffolding**: `cmd/agm-webhook-receiver`'s own header states it is not wired into a build target. It writes a bespoke JSONL file, not `pkg/eventbus` | `cmd/agm-webhook-receiver` |
| Mergeloop backpressure, cap-and-skip | **EXISTS and runs**, verified: `internal/mergeloop/driver.go:164-168`, default cap 50, `--cap` override; not emitted anywhere, only audited | `internal/mergeloop/driver.go`, `cmd/mergeloop/main.go` |
| Beads state, poll-based readiness | **EXISTS**, intentional per ADR-002, not event-driven, and the backing store is external to this repo | ADR-002, "Beads are the roadmap" |
| Absence and expectation primitive (Pulse plus alarm ladder) | **EXISTS**, unmerged, host-scoped by design, itself a poll not an event consumer | `pkg/absencealarm/pulse.go`, `state.go` (branch `stack/absence-alarm-cli`) |
| Cross-model, human-gated, staged pipeline (five stages) | **EXISTS**, one instance, not generalized | `research-pipeline/skills/research-pipeline/SKILL.md` |
| A unified event envelope every pipeline uses | **MISSING**. `pkg/eventbus.ChannelFromType` derives an event's channel from the first dot-segment of its type string; every event name proposed above (`pr.backpressure`, `bead.ready`, `paper.posted`) would resolve to a channel that is not one of the four defined ones and is therefore not durable | n/a |
| A place to declare "on event X, run pipeline Y" for anything other than an engram | **MISSING** | n/a |
| State transitions (bead ready, PR merged, escalation resolved) as emitted events | **MISSING**, confirmed gap | n/a |
| A reusable backpressure policy | **MISSING** (mergeloop's is real but single-purpose) | n/a |
| A merge decision that composes with ADR-031 and ADR-032 | **MISSING**, and out of scope for this document, see above | n/a |

The honest summary: dear-agent has built the pieces of this architecture at least six
separate times (VROOM's unwired topic constants, Wayfinder's tracker, the trigger
registry's engram-only wiring, absence-alarm's pulses, the webhook receiver's spike,
and `pkg/workflow`'s own audit trail) without generalizing any of them into one shape,
and without the cross-process transport question ever being asked directly. `agm-bus`
already answers that question for infrastructure-origin events; this proposal is
mostly a wiring and generalization exercise on top of what exists, not a
from-scratch build.

## Relationship to VROOM, Wayfinder, Beads, mergeloop

None of these get replaced. Each keeps its current job; this proposal adds a shared
event vocabulary underneath them.

VROOM stays the execution framework: three supervisors, polling for beads, the
Primary/Secondary/Tertiary model from CONTEXT.md. Its supervisors are LLM harness
sessions driven by `/loop` tick prompts, not Go processes with an event loop, so
"VROOM subscribes to an event" is not a small change; a more honest version of that
proposal is delivering a matched event's payload into a supervisor's next tick prompt
as pre-fetched context, which this document lists as a proposal to try and measure, not
a settled design.

Wayfinder stays the planning phase, unchanged. It already emits phase events into
`pkg/eventbus`; the gap is downstream and in the topic-name mismatch noted above, not
in Wayfinder itself.

Beads stays the roadmap and dependency graph, unchanged. ADR-002 is explicit that VROOM
does not own roadmap state, and beads' Dolt-backed store is a separate system this repo
consumes through the `bd` CLI, not one it can emit events from directly. Any bead-state
event this proposal adds is a differ inside dear-agent that watches `bd ready`'s output
change, which is itself polling with an extra step; this document names that honestly
rather than calling it push-based.

Mergeloop stays the merge driver, unchanged mechanically. The only change is making its
existing backpressure signal visible on `agm-bus` instead of only in its own audit log.

The research-pipeline skill becomes the first workflow the extended `pkg/workflow`
engine hosts as an event-triggered run, proving the wiring works on something real
instead of a toy example.

## Concrete proposals

Split into beads-sized changes (small, mechanical, independently shippable) and
SPEC-level proposals (need a design review before beads get filed against them,
because they touch shared infrastructure or governance). An earlier draft of this list
included four items as "beads-sized, one PR each" that in fact depend on the
cross-process transport question being answered first; this revision reorders around
that dependency and starts with the smallest real fix found during review.

### Beads-sized, independently shippable

Bead IDs below are from this repo's canonical Beads database
(`~/beads/context-engine/.beads`); look one up with `bd --db
~/beads/context-engine/.beads show <id>`.

0. **`ce-2uui5`** Fix the Wayfinder to trigger-registry topic-name mismatch: either the
   registry matches on the namespaced topic (`wayfinder.phase.started`) or Wayfinder's
   tracker emits the bare name the registry expects. This is a one-line-scale fix, it
   is a real defect today (any engram declaring `on: phase.started` is currently
   unreachable), and it proves the "unify the vocabulary" thesis on a concrete bug
   rather than a hypothetical one.
1. **`ce-f6iv2`** Make `pkg/trigger`'s subscriber derive its subscription set from the
   registry's registered event types, instead of the three hardcoded topics in
   `pkg/trigger/subscriber.go` today, so registering a new trigger for a new event type
   actually causes the router to listen for it.
2. **`ce-szpsa`** Add wildcard matching to `pkg/trigger/registry.go`'s lookup, aligned
   with `pkg/eventbus`'s existing pattern matcher (`local_bus.go`'s `matchesPattern`),
   so the two components agree on what a subscription pattern means.
3. **`ce-1jh17`** Emit `pr.backpressure(open, cap)` from mergeloop's existing cap check
   (`internal/mergeloop/driver.go:164-168`) over `agm-bus`, once proposal 5 below gives
   it somewhere durable to land (dependency recorded on the bead).

### SPEC-level, needs design review

4. **`ce-vyv35`** A unified event envelope, extending `pkg/eventbus.Event` with the
   provenance and related-ID fields VROOM's `HandedOffPayload` already models for its
   own topics (`FromRole`, `ToRole`, confidence), generalized so any pipeline can carry
   them, and with a channel-naming convention that does not silently drop every
   proposed event name into an undurable bucket, as `ChannelFromType` does today.
5. **`ce-0u1z7`** Cross-process delivery over `agm-bus` for any event that needs to
   reach a different process than the one that emitted it: mergeloop, absence-alarm,
   and any future pipeline runner all qualify. This is the answer to the transport
   question this document's own earlier draft left unasked; `agm-bus`'s existing
   durable per-session queue and ACL are the right foundation, not a new bus.
6. **`ce-o74or`** An event-triggered `pkg/workflow`, so a workflow run can be started
   by a trigger match instead of only a CLI invocation, hosting the research-pipeline
   skill as its first real workload. This replaces an earlier draft's proposal to
   build a new `pkg/pipeline` package from scratch; `pkg/workflow` already provides
   durable state, audit, and HITL that a new package would have to reinvent. Depends
   on proposals 4 and 5.
7. **`ce-4x2e3`** A generic backpressure policy package
   (`pkg/pipeline/backpressure`), `cap(N)`/`timeout(D)`/`staleness(D)`, extracted from
   mergeloop's proven implementation, usable by any workflow run or trigger match.
   Depends on proposal 6.
8. **`ce-vd8uq`** The merge decision from Flow B, designed on its own, against
   ADR-030's mechanical-classifier bar and ADR-032's must-reach-human taxonomy, not
   adopted as a side effect of proposals 4 through 7. See [§ Auto-merge
   illustration](#auto-merge-illustration-not-a-decision) above for why this document
   stops at a sketch.

## Open questions

1. `pkg/eventbus/local_bus.go` gives no ordering guarantee across handlers, even for a
   single event (`dispatchHandlers` fans out to one goroutine per handler); a producer
   using `Emit` gets no signal if every handler failed, since handler errors are
   swallowed; and the durable JSONL sink is write-only, nothing in the package reads it
   back. Any proposal that relies on the in-process bus for anything beyond
   best-effort, fire-and-forget fan-out inside one process needs to account for these
   properties explicitly, not assume them away.
2. Should event-driven pipelines be required to be deterministic (same input, same
   output), or is an LLM-driven transform stage, like Stage 3's plan-writing, fine as
   long as its output event is deterministic in shape?
3. Versioning: how does an event schema change without breaking a subscriber that has
   not been updated yet?
4. Is "deliver a matched event into VROOM's next tick prompt" actually a latency win
   over the current 60 to 180 second poll intervals, or does the poll cadence already
   make the difference immaterial? Measure before treating this as settled.

## References

- `pkg/eventbus/bus.go`, `pkg/eventbus/local_bus.go` (in-process event bus)
- `agm/internal/bus`, `agm/cmd/agm-bus`, `agm/internal/bus/emitter.go` (cross-process transport)
- `pkg/workflow` (durable workflow engine, ADR-010)
- `pkg/trigger/registry.go`, `subscriber.go`, `matcher.go` (trigger registry, engram-scoped today)
- `pkg/vroom/vroom/emitter.go`, `topics.go` (VROOM decision topic constants, unwired)
- `pkg/vroom/decisiontrail/trail.go` (append-only decision log)
- `pkg/vroom/escalation/doc.go`, `types.go` (escalation state machine)
- `wayfinder/internal/analytics/session_tracker.go` (Wayfinder phase events)
- `pkg/engram/trigger.go` (engram trigger spec, source of the topic-name mismatch)
- `internal/mergeloop/driver.go` (mergeloop backpressure)
- `cmd/agm-webhook-receiver` (GitHub webhook ingress, spike scaffolding)
- `pkg/absencealarm/pulse.go`, `state.go` (absence and expectation primitive, branch `stack/absence-alarm-cli`)
- `research-pipeline/skills/research-pipeline/SKILL.md` (five-stage cross-model pipeline)
- `docs/adr/ADR-002-vroom-execution-architecture.md` (VROOM topology, "Beads are the roadmap")
- `docs/adr/ADR-010-workflow-engine-architecture.md` (durable workflow engine)
- `docs/adr/ADR-029-ralph-wiggum-merge-loop.md` (mergeloop's host-tick design, why webhooks were rejected for it)
- `docs/adr/ADR-030-dependabot-auto-merge.md` (existing narrow, mechanical auto-merge precedent)
- `docs/adr/ADR-031-agent-escalation-path.md`, `ADR-032-escalate-to-supervisor.md` (must-reach-human taxonomy, exception-path discipline)
- C4 model: [c4model.com](https://c4model.com/), Context/Container/Component/Code levels
- Factorio Logistics Train Network: [wiki.factorio.com/Mods/Logistic_Train_Network](https://wiki.factorio.com/Mods/Logistic_Train_Network), provider/requester stations, dispatcher matching, network starvation
- Pipes-and-filters architectural style, e.g. Richards and Ford, *Fundamentals of Software Architecture*
