# Feedback-Loop Pipelines: Event/Trigger to Pipeline to Transform to Emit

<!-- Last audited at: 2026-09-02 -->

Status: Proposed. This document is a design proposal, not a description of a shipped
system. Every claim is marked EXISTING (running in a production binary today, cited),
NOT WIRED (the code exists but nothing in a production binary currently drives it), or
NEW (proposed, does not exist).

## The idea in one sentence

An event or trigger starts a pipeline; the pipeline transforms it and emits a new
event; that event can start the next pipeline. Some of these chains close into a
genuine feedback loop, where a later event changes an earlier decision; most, as
designed below, are event-triggered pipelines that do not close a loop on their own.
Both are useful. This document is careful about which is which, because dressing up
one as the other defeats the point of naming it.

## Diagrams

[feedback-loop-diagrams.md](feedback-loop-diagrams.md) has the System Context and
Container diagrams (C4, Mermaid) and two sequence diagrams for the example flows below.
Read this document first; the diagrams are the compressed version.

## What makes a loop a feedback loop

A feedback loop closes: some output is observed, that observation changes a future
decision, and the changed decision produces a new output that gets observed again. A
pipeline that only chains forward, event A triggers pipeline B triggers event C
triggers pipeline D, is event-triggered choreography. It is not a feedback loop unless
something eventually loops back and changes an earlier decision.

Two closed feedback loops already exist in this codebase, and they are the right
reference point for what this document is and is not adding:

- The absence-alarm's alarm-then-recovery cycle (`pkg/absencealarm`): a pulse goes
  silent, an alarm fires, a human or process corrects it, the pulse resumes, the alarm
  clears. State observed, correction made, state re-observed. Closed.
- DEAR, the retrospective loop named in this repository's own `CONTEXT.md`: findings
  from finished work feed back into the roadmap through the Meta-Orchestrator, changing
  what gets planned next. Closed.

Neither of the two example flows below closes a loop by itself. Flow A is a ticket
triggering a pipeline that produces a PR; nothing measures the PR's outcome and feeds
it back into how future tickets get triaged. Flow B produces a benchmark delta and
routes it to a merge decision, but nothing in this document feeds that delta back into
which papers get researched next or how future candidates get generated. Both are
named accurately below as event-triggered pipelines, not feedback loops, because that
is what they are.

The one place this document proposes a loop that actually closes is narrower and more
concrete: mergeloop's backpressure signal, once visible outside its own audit log
(see [§ Backpressure](#backpressure-and-throughput) below), is a measurement a future
intake decision could react to. This document proposes making that signal visible; it
does not yet propose the throttling policy that would read it and close the loop. That
gap is named explicitly in the open questions.

## The model

```
  EVENT / TRIGGER  ->  PIPELINE  ->  TRANSFORM(S)  ->  EMIT NEW EVENT  ->  (maybe loops back)
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
- **Emit**: the pipeline's output is itself a new event, not just a side effect. A
  pipeline that writes a file and stops is a transform. A pipeline that writes a file
  and emits `plan.approved` so the next pipeline can pick it up is event-triggered
  choreography, and becomes a feedback loop only if something downstream eventually
  changes an earlier decision because of it.

## Event taxonomy

| Trigger class | Examples | Today | Proposed |
|---|---|---|---|
| **External ingress** | GitHub webhook (PR opened, check completed), arXiv/RSS new paper | `cmd/agm-webhook-receiver` writes a bespoke JSONL file; compiled and tested by `go build ./...` but not installed, deployed, or connected to any bus (NOT WIRED) | Generalize: any external source lands as a typed event on the trigger router, not a bespoke file per source |
| **Scheduled tick** | launchd cron: mergeloop, absence-alarm, disk-watchdog, sandbox-gc | Each tool polls on its own interval, independently (EXISTING) | Ticks stay ticks; they are triggers too. But a tick's findings (a PR queue backed up) become an emitted event other pipelines can react to, not just a log line |
| **State transition** | Bead becomes ready, blocked, or closed; PR merged; escalation resolved | Polled, not emitted (confirmed gap, see [§ What already exists](#what-already-exists)) | State transitions become emitted events (`bead.ready`, `pr.merged`, `escalation.resolved`) alongside the existing state write, not instead of it |
| **Agent-raised** | Worker raises a process problem, an agent proposes a process improvement | `agm escalate` (EXISTING, for blocking questions and decisions; routed through `pkg/vroom/escalation`'s classifier, ADR-031, ADR-032) | Extend to non-blocking "I noticed X" events that do not require a human answer to proceed. Today escalation assumes something is blocked; a proposal is not |
| **Absence** | Expected event did not arrive: a merge, a heartbeat, a sweep | `pkg/absencealarm` (a periodic host state probe: file mtime, launchd job presence, command exit; not itself event-driven, see [§ Absence-alarm](#absence-alarm-is-a-closed-loop-already-not-an-event-consumer)) | Wire its alarm and recovery transitions onto `agm-bus`, once it has a durable publish path, so "the mergeloop went dark" is the same kind of signal as "a PR merged," searchable in the same place |
| **Pipeline output** | A pipeline's own emitted event | Ad hoc per pipeline (Wayfinder's phase events are the clearest real example, and even they land in a short-lived, unsubscribed, non-durable channel today) | Standardize the envelope so any pipeline's output can trigger any other pipeline, not only the one it was written for |

## The Factorio Logistics Train Network analogy

Valentin's reference point, made explicit because it is a genuinely good structural
match, not just a fun comparison, and because getting the mechanics right matters more
than the analogy being memorable.

In Factorio's LTN mod, stations declare what they have (provide) or need (request) as
item and quantity thresholds. The mod does not move anything itself; it watches the
declared requests and provides, and when a request can be matched to a provider with
enough stock and a free train, it dispatches a train to carry that specific cargo
between those two specific stations. A request that cannot be matched, because no
provider clears its threshold or no train is free, does not disappear silently: LTN
logs a line for it (an unmatched request, no train found) at its configured message
level, but that line scrolls past in a console like any other log output unless
something is specifically watching for it.

| LTN concept | This architecture |
|---|---|
| A station's request ("I need 200 iron plates") | An event a pipeline is waiting on, a trigger subscription |
| A station's provide ("I have 500 copper ready") | A pipeline's emitted event, something now available for the next stage |
| The LTN dispatcher matching requests to provides | The trigger router matching emitted events to subscribed pipelines |
| A train carrying cargo between two stations | A worker, an agent session executing one pipeline run |
| Per-station train limit | Concurrency limit on how many pipeline runs can be in flight at once |
| Provide/request stack threshold (minimum cargo per dispatch) | Batching granularity: how large a request has to get before it is worth dispatching a run for, the "fewer, larger requests" relief lever discussed below |
| An unmatched request's log line, present but easy to miss | Our actual, lived problem: mergeloop's backpressure check writes to its own audit log today, present but easy to miss (see [§ Backpressure](#backpressure-and-throughput)) |
| A station that stops requesting or providing and nobody notices | What the absence-alarm exists to catch |

The analogy earns its keep on that last row: an LTN signal that scrolls past in a
console is functionally the same failure mode as mergeloop's backpressure audit log
entry that nobody is tailing. Both are present, both are technically observable, and
both are easy to miss until the failure has compounded for days. Making that signal
land somewhere it can be watched, rather than inventing a signal that does not exist
today, is what this proposal is for.

## Example Flow A: simple, ticket to assigned worker

See the sequence diagram in [feedback-loop-diagrams.md, Flow
A](feedback-loop-diagrams.md#3-flow-a-simple-ticket-to-assigned-worker-as-a-sequence).
This flow is event-triggered choreography, not a feedback loop; see
[§ What makes a loop a feedback loop](#what-makes-a-loop-a-feedback-loop) above.

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

The flow deliberately stops there. Mergeloop's existing 10-minute tick discovers the
new PR on its own next pass regardless, so emitting an event here would carry no
information mergeloop's tick does not already get, and ADR-029 already chose a
host-tick design over webhooks for mergeloop for reasons this proposal is not trying to
relitigate. Flow A is the baseline case Flow B is advanced relative to; it is not
itself an argument for anything new.

## Example Flow B: advanced, arXiv paper to a merge decision

See the sequence diagram in [feedback-loop-diagrams.md, Flow
B](feedback-loop-diagrams.md#4-flow-b-advanced-arxiv-paper-to-a-merge-decision). Like
Flow A, this is event-triggered choreography; see [§ What makes a loop a feedback
loop](#what-makes-a-loop-a-feedback-loop) above for what it would take to close it.

1. An RSS or arXiv feed poster emits `paper.posted`.
2. The trigger router matches it to the research-pipeline skill (EXISTING,
   `research-pipeline/skills/research-pipeline/SKILL.md`), started as a workflow run:
   Stage 2 does goal-oriented research on how the paper's idea might apply to
   dear-agent; Stage 3 hands off to an independent model that adversarially
   fact-checks the research and writes a codebase-grounded plan.
3. The plan hits the skill's existing human review gate: Stage 3 to Stage 4, with the
   approval receipt bound to the exact plan revision (commit SHA or content digest), as
   the skill already requires.
4. Stage 4 runs next, unchanged from the skill: a third independent model
   adversarially reviews the approved plan before any bead exists. If it finds a
   blocking defect, the plan goes back to Stage 3's author to fix, then Stage 4
   re-reviews, with a fresh approval receipt bound to the corrected revision. Once it
   ships unconditionally, Stage 4's actual output is decomposition into sized beads,
   one per candidate approach, filed through the portable Beads interface exactly as
   the skill specifies, not a direct spawn of worker agents.
5. Those candidate beads become ready and VROOM dispatches workers to them through the
   same poll-based path Flow A uses. Routing through beads rather than spawning N
   workers directly means VROOM's own dispatch capacity, not a separate check, is what
   keeps every candidate from appearing on the PR queue at the same moment; a
   backpressure check would otherwise have to duplicate a decision VROOM already makes.
6. Each candidate PR triggers `benchmark.requested`. A backpressure policy, generalized
   from mergeloop's cap, admits, queues, or rejects the benchmark run based on current
   concurrent-benchmark count: the LTN "is a train free" check.
7. All N candidates are measured against one shared, independently authored benchmark,
   not one each candidate writes for itself. The research-pipeline skill's own rule for
   Stage 3 and Stage 4 review, never let one model mark its own homework, applies here
   too: a benchmark authored by the same lineage that wrote the change it measures is
   not independent.
8. The benchmark pipeline emits `benchmark.completed(pr, delta)`. The delta is the
   payload that makes the next stage's decision cheap: it does not have to go
   re-measure anything.
9. A merge decision evaluates the delta against the change's risk classification. This
   document sketches, but does not ship, that decision; see the next section for why.

## Auto-merge illustration, not a decision

Flow B's diagram shows where a merge decision would plug in, and every concrete path
out of it in that diagram routes to human review. A decision table deciding when a
benchmark-confirmed change auto-merges without a human is deliberately not included
here. Real, concrete gaps make that the right call, not just caution for its own sake:

- Any such table needs to exclude its own thresholds and safety constants from its own
  "auto-merge eligible" class. `internal/mergeloop/driver.go`'s cap, `cmd/vroom-dispatch`'s
  spawn-interval circuit breaker, and the absence-alarm's clock-skew tolerance are all
  single-package constants a benchmark could show "improved" while quietly disabling
  the safety property they exist to enforce.
- The benchmark measuring a change must be independent of the change's own author
  lineage (see Flow B step 7); a table that does not enforce this lets an agent's PR
  and its own measurement come from the same source.
- "Same shape as an already-accepted auto-merge class" cannot be left to agent
  judgment with no expiry or re-audit. ADR-030's actual precedent is a mechanical
  classifier (bot identity, semver bump type, immutable head binding), not a judgment
  call, and a table claiming the same guarantee needs the same mechanical bound.
- A path check for "foundational" (did the diff touch `SPEC.md`, `CONTEXT.md`, an ADR,
  or a public interface) misses a change that contradicts an existing SPEC
  requirement's behavior without editing the SPEC file, which is exactly the shape of
  change worth catching.
- A merge decision needs to compose with `pkg/vroom/escalation`'s existing
  must-reach-human taxonomy (ADR-032: product, pricing, security, destructive, legal,
  spend, people, and external-communication decisions) and with ADR-031's rule that
  repeated use of an exception path is itself a defect signal, not a separate
  taxonomy invented from scratch.

Each of these is a real path by which an agent's own change could ship without a human
catching it. The merge decision needs the same scrutiny ADR-030 got, worked out on its
own, not adopted as a side effect of the plumbing proposed here.

## Backpressure and throughput

Two things need to be true for the LTN analogy to help operationally, not just
narratively.

First, a queue depth must be visible at the point of contention, not buried in one
tool's private log. Today, mergeloop's cap-exceeded check
(`internal/mergeloop/driver.go:164-168`) is exactly the right signal, generated in
exactly the right place, but it only reaches mergeloop's own audit log; nothing
downstream can react to it. Proposal: the same check emits `pr.backpressure(open, cap)`
once `agm-bus` has a durable path to carry it (see [§ What already
exists](#what-already-exists) below for why that is not yet true). That single change
makes the PR-queue throughput problem observable in the same place as everything else,
instead of something a human has to notice by reading mergeloop's audit file.

Second, backpressure must be a declarable policy, not a per-tool special case.
Mergeloop's cap-and-skip logic is real and proven. Nothing else in the repo has an
equivalent. The proposed backpressure policy (`pkg/pipeline/backpressure`, new)
extracts that proven cap(N) mechanism into something any pipeline can declare; a
timeout(D) and staleness(D) form modeled on the same shape are new designs this
proposal introduces, not extractions of anything already proven, and should be treated
with that lower confidence until a second real consumer exists.

Relief, in LTN terms, is either adding capacity (more trains: more review bandwidth,
more worker slots) or reducing demand (fewer, larger requests: batch tickets, raise the
bar for what auto-triggers a pipeline). This document does not propose which lever to
pull in general; it proposes that the queue depth be visible enough that pulling one is
a decision, not a discovery three weeks later. It also does not yet propose the
throttling policy that would read `pr.backpressure` and change intake behavior
automatically; that is the concrete next step that would turn this signal into a closed
feedback loop, named as an open question below rather than assumed.

## Absence-alarm is a closed loop already, not an event consumer

`pkg/absencealarm` and `cmd/absence-alarm` (branch `stack/absence-alarm-cli`, stacked
on `stack/absence-alarm-domain`, committed but not yet in `main`) inverts the rest of
this document: instead of "event X arrived, run a pipeline," it is "event X was
expected and did not arrive, alarm." Its `Pulse` primitive is a periodically probed
host state check (a file's modification time, whether a launchd label appears in the
job listing, whether a command exits zero), each with a maximum silence window. The
probe itself is a poll, not an event consumer. But the full cycle, silence detected,
alarm raised, correction made, pulse resumes, alarm clears, is a genuine closed
feedback loop, and one of only two that already exist in this codebase (the other is
DEAR; see [§ What makes a loop a feedback loop](#what-makes-a-loop-a-feedback-loop)
above).

This proposal does not fold absence-alarm into the trigger router's model. It stays a
purpose-built host-level watchdog, deliberately independent of the mesh, by design: its
own SPEC names "the watcher lives inside the thing being watched" as the exact
anti-pattern it exists to avoid, and wiring it as just another subscriber would
reintroduce that failure mode. What this proposal does ask for is narrower:
absence-alarm's alarm and recovery transitions should also land on `agm-bus`
(`pulse.absent`, `pulse.recovered`) once that transport has a durable publish path, so
an operator or another pipeline can see "the mergeloop went dark" as the same kind of
signal as "a PR merged," without absence-alarm depending on that transport being alive
to detect that the transport itself might be the thing that is dark.

## What already exists

This is the load-bearing distinction the proposals below depend on. Every claim below
was checked against the actual source in this worktree.

| Piece | Status | Where |
|---|---|---|
| In-process event bus (four channels, Subscribe/Emit/AddSink, typed `Event`) | **EXISTS**, in-process only, `pkg/eventbus/local_bus.go:17` documents itself as "an in-process event bus implementation" | `pkg/eventbus/bus.go`, `pkg/eventbus/local_bus.go` |
| Cross-process session-addressed message broker | **EXISTS**, but with no publish/subscribe frame type. `agm/internal/bus/wire.go` defines nine frame types (`hello`, `welcome`, `send`, `deliver`, `ack`, `error`, `permission_request`, `permission_verdict`, `bye`), every one addressed to a single session id. `Emitter.EmitEvent`'s broadcast convenience sends a `send` frame with an empty target, which `Frame.Validate()` rejects (`"send: missing to"`); the client never reads the server's response, so the rejection is silent. The one production caller, the broker's own heartbeat watcher, appears to be hitting this today | `agm/internal/bus`, `agm/cmd/agm-bus`, `agm/internal/bus/wire.go`, `agm/internal/bus/emitter.go`, `agm/internal/bus/heartbeat_watcher.go` |
| Durable workflow engine (YAML dependency-ordered nodes, SQLite state, per-transition audit, HITL) | **EXISTS**, general-purpose, already invokable from the CLI (`cmd/workflow-run`) and MCP (`cmd/dear-agent-mcp`'s `workflow_run` tool), not event-triggered | `pkg/workflow` (ADR-010), `cmd/workflow-run`, `cmd/dear-agent-mcp/workflow.go` |
| Trigger registry (event type to engram dispatch) and its subscriber | **Implemented and tested, zero production callers.** `pkg/trigger/subscriber.go`'s `TriggerSubscriber.Start` subscribes to two wildcard patterns and one exact topic on `pkg/eventbus`, but has no caller outside its own test file. The only production consumer of `pkg/trigger` at all is the `engram trigger` CLI, which builds a registry and matches one synthetic event passed as an argument; it never touches a `LocalBus`. Matching is exact-match only (`pkg/trigger/registry.go`'s `Lookup` is a plain map keyed on the literal event type string) | `pkg/trigger/registry.go`, `subscriber.go`, `matcher.go`, `engram/cmd/engram/cmd/trigger.go` |
| Precedent for a cross-process bridge package | **EXISTS**: `agm/workflowbus/bridge.go` connects `pkg/workflow` (outside `agm/`) to `agm/internal/bus`, and its own doc comment states why it lives under `agm/`: `agm/internal/bus` is only importable from `agm/`, by Go's internal-package rule. The same shape is the answer for `pkg/trigger`, which has the identical constraint | `agm/workflowbus/bridge.go` |
| VROOM decision topic constants and an `Emitter` type | **Defined, zero production callers.** `EmitDispatched`/`EmitEscalated`/`EmitEvaluated`/`EmitGated`/`EmitHandedOff` have no non-test call sites anywhere in the repo; the decision trail (below) is what actually runs | `pkg/vroom/vroom/emitter.go`, `topics.go` |
| Append-only decision log (JSONL) | **EXISTS**, fragmented across separate implementations (decision trail, escalation log, webhook receiver's own file) | `pkg/vroom/decisiontrail/trail.go`, `pkg/vroom/escalation` |
| Wayfinder phase events | **EXISTS as an emit, not reachable.** `wayfinder-session start-phase` and `complete-phase` each construct a fresh `LocalBus` (`wayfinder/cmd/wayfinder-session/internal/tracker/tracker.go:82`), emit one event under the namespaced topics `wayfinder.phase.started`/`wayfinder.phase.completed`, and exit; no subscriber is ever attached to that bus in that process, and the `wayfinder` channel `ChannelFromType` derives from the topic prefix is not one of the four durable channels. A third naming variant, the bare `phase.started` in `wayfinder/cmd/wayfinder-session/internal/history/types.go`, and a fourth, `notification.phase.complete` in `pkg/eventbus/bus.go`, exist unused alongside it, none matching what `pkg/engram/trigger.go`'s documented `TriggerSpec.On` values or the trigger registry's exact-match lookup expect | `wayfinder/cmd/wayfinder-session/internal/tracker/tracker.go`, `wayfinder/internal/analytics/session_tracker.go`, `pkg/engram/trigger.go`, `pkg/trigger/registry.go` |
| GitHub webhook ingress | **Compiled, not deployed.** `cmd/agm-webhook-receiver` is `package main` and is built and tested by the repository's `go build ./...` and test gates, but has no Makefile target, deploy path, or launchd job, and writes a bespoke JSONL file rather than publishing to any bus | `cmd/agm-webhook-receiver` |
| Mergeloop backpressure, cap-and-skip | **EXISTS and runs**, verified: `internal/mergeloop/driver.go:164-168`, default cap 50, `--cap` override; not emitted anywhere, only audited | `internal/mergeloop/driver.go`, `cmd/mergeloop/main.go` |
| Beads state, poll-based readiness | **EXISTS**, intentional per ADR-002, not event-driven, and the backing store is external to this repo | ADR-002, "Beads are the roadmap" |
| Absence and expectation primitive, closed loop | **EXISTS**, committed but unmerged, host-scoped by design; the probe is a poll, but the full alarm-then-recovery cycle is a real closed feedback loop | `pkg/absencealarm/pulse.go`, `state.go` (branch `stack/absence-alarm-cli`) |
| Cross-model, human-gated, staged pipeline (five stages) | **EXISTS**, one instance, not generalized. Stage 4's real output is decomposition into sized beads through the portable Beads interface, not a direct worker spawn | `research-pipeline/skills/research-pipeline/SKILL.md` |
| Notification adapters | **EXISTS**: Discord and Matrix, on `agm-bus`. No desktop-notification implementation was found under `agm/internal/bus` or `agm/cmd/agm-bus` | `agm/internal/bus` channel adapters |
| A unified event envelope every pipeline uses | **MISSING**. `pkg/eventbus.ChannelFromType` derives an event's channel from the first dot-segment of its type string; every event name proposed above (`pr.backpressure`, `bead.ready`, `paper.posted`) would resolve to a channel that is not one of the four defined ones and is therefore not durable, the same failure Wayfinder's own events already have today | n/a |
| A publish/subscribe primitive on `agm-bus` | **MISSING**. See the broker row above; this is a wire-protocol addition, not a wiring exercise | n/a |
| A place to declare "on event X, run pipeline Y" for anything other than an engram, actually running in a process | **MISSING** | n/a |
| State transitions (bead ready, PR merged, escalation resolved) as emitted events | **MISSING**, confirmed gap | n/a |
| A reusable backpressure policy | **MISSING** (mergeloop's cap-and-skip is real and proven; timeout and staleness forms are new) | n/a |
| A merge decision that composes with ADR-031 and ADR-032 | **MISSING**, and out of scope for this document, see above | n/a |

The honest summary: dear-agent has built the pieces of this architecture at least six
separate times (VROOM's unwired topic constants, Wayfinder's tracker, the trigger
registry's untriggered wiring, absence-alarm's pulses, the webhook receiver's
uncompiled path, and `pkg/workflow`'s own audit trail) without generalizing any of
them into one shape, and the cross-process transport question has a real answer,
`agm-bus`, that itself needs a protocol addition before it can carry these events. This
proposal is a wiring, generalization, and one real protocol extension, not a
from-scratch build of a new bus.

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

Wayfinder stays the planning phase, unchanged. It already emits phase events; the gap
is that nothing is listening in the process that emits them, and its topic naming has
drifted across at least three variants, not that Wayfinder itself needs to change.

Beads stays the roadmap and dependency graph, unchanged. ADR-002 is explicit that VROOM
does not own roadmap state, and beads' Dolt-backed store is a separate system this repo
consumes through the `bd` CLI, not one it can emit events from directly. Any bead-state
event this proposal adds is a differ inside dear-agent that watches `bd ready`'s output
change, which is itself a poll with an extra step; naming that honestly here rather
than calling it push-based.

Mergeloop stays the merge driver, unchanged mechanically. The only change is making its
existing backpressure signal visible on `agm-bus` instead of only in its own audit log,
once that transport can carry it.

The research-pipeline skill's Stage 4 decomposition, into sized beads through the
portable Beads interface, becomes the first workload the extended `pkg/workflow`
engine hosts as an event-triggered run, dispatched the same way any other bead is.

## Concrete proposals

Split into beads-sized changes (small, mechanical, independently shippable) and
SPEC-level proposals (need a design review before beads get filed against them,
because they touch shared infrastructure or governance).

### Beads-sized, independently shippable

Bead IDs below are from this repo's canonical Beads database
(`~/beads/context-engine/.beads`); look one up with `bd --db
~/beads/context-engine/.beads show <id>`. None of these beads alone makes an engram
fire end to end; each is a small, independently testable piece of the larger SPEC-level
work in items 4 through 8, and their acceptance criteria are scoped to what they
individually prove, not to the full pipeline being reachable.

0. **`ce-2uui5`** Fix the Wayfinder to trigger-registry topic-name drift: pick one
   naming convention (namespaced, `wayfinder.phase.started`, is recommended, since it
   is what actually gets emitted today) and correct the registry's expected string, the
   documented `TriggerSpec.On` values, and the two other unused naming variants to
   match. This closes one of several independent gaps blocking a trigger from firing;
   see proposal 6 for the others (production composition, the subscribe primitive on
   `agm-bus`).
1. **`ce-f6iv2`** Make `pkg/trigger`'s subscriber derive its subscription set from the
   registry's registered event types, instead of the two wildcard patterns and one
   exact topic hardcoded in `pkg/trigger/subscriber.go` today, so registering a new
   trigger for a new event type actually causes the router to listen for it once the
   subscriber is running somewhere (see proposal 6).
2. **`ce-szpsa`** Add wildcard matching to `pkg/trigger/registry.go`'s lookup, aligned
   with `pkg/eventbus`'s existing pattern matcher (`local_bus.go`'s `matchesPattern`),
   so the two components agree on what a subscription pattern means.
3. **`ce-1jh17`** Emit `pr.backpressure(open, cap)` from mergeloop's existing cap check
   (`internal/mergeloop/driver.go:164-168`) over `agm-bus`, once the unified envelope
   (proposal 4) and the publish/subscribe primitive (proposal 5) both land.

### SPEC-level, needs design review

4. **`ce-vyv35`** A unified event envelope, extending `pkg/eventbus.Event` with the
   provenance and related-ID fields VROOM's `HandedOffPayload` already models for its
   own topics (`FromRole`, `ToRole`, confidence), generalized so any pipeline can carry
   them, and with a channel-naming convention that does not silently drop every
   proposed event name into an undurable bucket, as `ChannelFromType` does today.
5. **`ce-0u1z7`** A publish/subscribe primitive on `agm-bus`. This is a wire-protocol
   addition, not a wiring exercise: today's nine frame types are all addressed to one
   session id, and the `EmitEvent` broadcast convenience is rejected by the server's
   own frame validation. The design needs a topic-addressed frame type, a durable
   per-topic queue (mirroring the existing per-session offline queue's durability
   guarantees), and a fix or replacement for `EmitEvent` so its existing production
   caller, the heartbeat watcher, actually delivers.
6. **`ce-o74or`** A bridge package under `agm/`, mirroring `agm/workflowbus/bridge.go`,
   that subscribes to `agm-bus` (once proposal 5 lands), matches events through
   `pkg/trigger`'s registry, and starts `pkg/workflow` runs. This is the single
   long-lived process that resolves the trigger subscriber's "implemented, never
   instantiated" gap and the "`pkg/trigger` cannot import `agm/internal/bus`" Go
   visibility constraint at the same time, since a bridge under `agm/` can import both
   `agm/internal/bus` and the public `pkg/trigger` and `pkg/workflow` packages. It also
   needs a typed trigger-action model: "a trigger match starts a workflow run" is not
   yet an interface, and needs an explicit contract for the event-to-workflow input
   mapping, authorization, idempotency, and failure/retry ownership before it is
   implementable. Depends on proposals 4 and 5.
7. **`ce-4x2e3`** A generic backpressure policy package
   (`pkg/pipeline/backpressure`), extracting mergeloop's proven `cap(N)` mechanism;
   `timeout(D)` and `staleness(D)` are new designs modeled on the same shape, not yet
   proven, and should carry that caveat until a second real consumer exists. Depends on
   proposal 6.
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
5. What would it take to close the loop this document identifies but does not build:
   `pr.backpressure` feeding into an actual intake-throttling decision, so a busy queue
   changes future trigger-router admission rather than only becoming visible? Naming
   the controller and the state it would read is the next design step, not this one.

## References

- `pkg/eventbus/bus.go`, `pkg/eventbus/local_bus.go` (in-process event bus)
- `agm/internal/bus`, `agm/cmd/agm-bus`, `agm/internal/bus/wire.go`, `emitter.go`, `heartbeat_watcher.go` (cross-process broker and its current protocol limits)
- `agm/workflowbus/bridge.go` (precedent for a bridge package under `agm/`)
- `pkg/workflow` (durable workflow engine, ADR-010), `cmd/workflow-run`, `cmd/dear-agent-mcp/workflow.go` (existing non-CLI invocation surfaces)
- `pkg/trigger/registry.go`, `subscriber.go`, `matcher.go`, `engram/cmd/engram/cmd/trigger.go` (trigger registry, its only production consumer)
- `pkg/vroom/vroom/emitter.go`, `topics.go` (VROOM decision topic constants, unwired)
- `pkg/vroom/decisiontrail/trail.go` (append-only decision log)
- `pkg/vroom/escalation/doc.go`, `types.go` (escalation state machine)
- `wayfinder/cmd/wayfinder-session/internal/tracker/tracker.go`, `wayfinder/internal/analytics/session_tracker.go` (Wayfinder phase events)
- `pkg/engram/trigger.go` (engram trigger spec, source of the topic-name mismatch)
- `internal/mergeloop/driver.go` (mergeloop backpressure)
- `cmd/agm-webhook-receiver` (GitHub webhook ingress, compiled, not deployed)
- `pkg/absencealarm/pulse.go`, `state.go` (branch `stack/absence-alarm-cli`, a closed feedback loop already)
- `research-pipeline/skills/research-pipeline/SKILL.md` (five-stage cross-model pipeline)
- `docs/adr/ADR-002-vroom-execution-architecture.md` (VROOM topology, "Beads are the roadmap")
- `docs/adr/ADR-010-workflow-engine-architecture.md` (durable workflow engine)
- `docs/adr/ADR-029-ralph-wiggum-merge-loop.md` (mergeloop's host-tick design, why webhooks were rejected for it)
- `docs/adr/ADR-030-dependabot-auto-merge.md` (existing narrow, mechanical auto-merge precedent)
- `docs/adr/ADR-031-agent-escalation-path.md`, `ADR-032-escalate-to-supervisor.md` (must-reach-human taxonomy, exception-path discipline)
- `CONTEXT.md` (the DEAR retrospective loop, and the VROOM/AGM/Wayfinder vocabulary this document assumes)
- C4 model: [c4model.com](https://c4model.com/), Context/Container/Component/Code levels
- Factorio Logistics Train Network: mod portal and wiki documentation of provider/requester stations, dispatcher matching, and unmatched-request logging
- Pipes-and-filters architectural style, e.g. Richards and Ford, *Fundamentals of Software Architecture*
