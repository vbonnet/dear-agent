# Feedback-Loop Architecture: Diagrams

<!-- Last audited at: 2026-09-02 -->

Companion diagrams for [feedback-loop-pipelines.md](feedback-loop-pipelines.md). Two
levels use the C4 model (System Context, Container); the two example flows are
expressed as sequence diagrams instead of C4 Component diagrams, because a flow is a
temporal chain of events across several containers, not the internal structure of one
container. All diagrams are Mermaid, in fenced code blocks, and render natively on
GitHub and in Claude Artifacts.

Every `Rel()` and message carries one of three labels: EXISTING (the wiring runs in a
production binary today), NEW (proposed, does not exist), or NOT WIRED (the code
exists but nothing in a production binary currently drives it). See
[feedback-loop-pipelines.md, "What already
exists"](feedback-loop-pipelines.md#what-already-exists) for the file:line citation
behind each one.

## 1. System Context

Who talks to the feedback-loop system, and why. `Trigger Router + Event-Triggered
Workflow Runs` is the proposed addition; everything else already exists.

```mermaid
C4Context
    title System Context: dear-agent feedback-loop system

    Person(operator, "Operator", "Valentin: approves gates, reads escalations")
    System(feedback, "Event/Trigger/Pipeline System", "Trigger Router over agm-bus, driving event-triggered pkg/workflow runs")

    System_Ext(github, "GitHub", "Issues, PRs, Actions checks")
    System_Ext(feeds, "arXiv / RSS feeds", "New research papers")
    System_Ext(cron, "launchd ticks", "mergeloop, disk-watchdog, absence-alarm, sandbox-gc")
    System_Ext(agents, "Agents (VROOM, Wayfinder, workers)", "Raise process problems, propose improvements, do the work")
    SystemDb_Ext(beads, "Beads", "Dolt-backed work-item graph, ~/beads/context-engine/.beads")

    Rel(github, feedback, "ticket created, PR opened or updated", "webhook + poll, NEW")
    Rel(feeds, feedback, "new paper posted", "RSS poll, NEW")
    Rel(cron, feedback, "scheduled tick", "launchd, EXISTING")
    Rel(agents, feedback, "raises problem, proposes improvement", "escalation, decision trail, EXISTING")
    Rel(beads, feedback, "bead ready, blocked, closed", "state change, NEW as an emitted event, today it is polled")
    Rel(feedback, github, "opens PR, comments, merges", "safe-* / GitHub API, EXISTING")
    Rel(feedback, beads, "creates and updates beads", "bd CLI, EXISTING")
    Rel(feedback, operator, "notifies, requests review", "Discord/Matrix, PR review request, EXISTING adapters")
    Rel(operator, feedback, "approves gate, answers escalation", "PR review, chat, EXISTING")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## 2. Container

Inside the system boundary. The cross-process transport question matters here:
`pkg/eventbus.LocalBus` is documented as "an in-process event bus implementation"
(`pkg/eventbus/local_bus.go:17`); it does not cross a process boundary on its own.
Every trigger in this document's example flows originates in a different process from
the one that should react to it (mergeloop and absence-alarm are separate launchd
one-shots; VROOM supervisors are separate harness sessions).

`agm-bus` (`agm/internal/bus`, `agm/cmd/agm-bus`) is the durable cross-process unix
socket broker this design routes trigger events over, but its wire protocol today has
no publish/subscribe primitive. Its nine frame types are `hello`, `welcome`, `send`,
`deliver`, `ack`, `error`, `permission_request`, `permission_verdict`, and `bye`
(`agm/internal/bus/wire.go`), all addressed to one session id. The convenience helper
that looks like a broadcast, `Emitter.EmitEvent`, sends a `send` frame with an empty
target; the server's `Frame.Validate()` rejects any `send` frame with an empty `To`
(`agm/internal/bus/wire.go`, `"send: missing to"`), and the client-side `Emitter`
never reads the server's response, so the rejection is invisible to the caller. The
one production caller of `EmitEvent`, the broker's own heartbeat watcher
(`agm/internal/bus/heartbeat_watcher.go`), is therefore calling a path that appears to
be silently rejected today. Because `pkg/trigger` lives at
`github.com/vbonnet/dear-agent/pkg/trigger`, outside the `agm/` prefix, it cannot
import `agm/internal/bus` directly under Go's internal-package rule; a bridge package
under `agm/` is required, the same shape `agm/workflowbus/bridge.go` already uses to
let `pkg/workflow` (also outside `agm/`) reach the same broker. This diagram shows the
proposed bridge as part of the Trigger Router container's own scope rather than as a
separate box, since it is one implementation unit: a long-lived process under `agm/`
that subscribes to `agm-bus`, matches events through `pkg/trigger`'s registry, and
starts `pkg/workflow` runs.

```mermaid
C4Container
    title Container: Event/Trigger/Pipeline System

    Person(operator, "Operator")

    System_Boundary(feedback, "Event/Trigger/Pipeline System") {
        Container_Ext(agmbus, "agm-bus", "Go, agm/internal/bus + agm/cmd/agm-bus", "EXISTING as a session-addressed message broker. Durable per-session offline queue, ACL. No publish/subscribe frame type; EmitEvent's broadcast convenience is rejected by Frame.Validate() today")
        Container(router, "Trigger Router + Bridge", "Go, pkg/trigger extended, new bridge package under agm/", "pkg/trigger's registry and matcher are implemented and tested but have zero production callers outside a CLI (NOT WIRED). Proposed: a new agm/-rooted bridge process, mirroring agm/workflowbus/bridge.go, that subscribes to agm-bus, matches through the registry, and starts workflow runs")
        Container_Ext(workflow, "Workflow Engine", "Go, pkg/workflow", "EXISTING (ADR-010). YAML dependency-ordered runs, SQLite state, per-transition audit, first-class HITL. Already startable from the CLI (cmd/workflow-run) and MCP (cmd/dear-agent-mcp's workflow_run tool); the proposed addition is starting a run from a trigger match, not a new invocation surface")
        Container(backpressure, "Backpressure Policy", "Go, new pkg/pipeline/backpressure", "NEW. Extracts mergeloop's proven cap(N) cap-and-skip; timeout(D) and staleness(D) are new policy designs modeled on the same shape, not yet independently proven")
    }

    Container_Ext(eventbus, "Event Bus (in-process)", "Go, pkg/eventbus", "EXISTING. Four channels, typed Event, Subscribe/Emit/AddSink. Fans out within one running process; does not itself cross a process boundary")
    Container_Ext(absencealarm, "Absence Alarm", "Go, cmd/absence-alarm", "Committed on branch stack/absence-alarm-cli, not yet in main. Pulse registry, scheduler, alarm sink. Alarms when an expected periodic signal (file mtime, launchd job presence, command exit) goes stale; probes are polled, and the alarm-then-recovery cycle is a genuine closed feedback loop already, independent of everything else in this diagram")
    Container_Ext(vroom, "VROOM Supervisors", "Go, cmd/vroom-dispatch", "EXISTING. Three supervisor harness sessions on independent tick timers (Meta-Orchestrator 180s, Orchestrator 90s, Overseer 60s), driven by /loop prompts, not an event loop")
    Container_Ext(mergeloop, "Mergeloop", "Go, cmd/mergeloop", "EXISTING. 10-minute launchd tick, cap-based backpressure, audits but does not emit its findings")
    Container_Ext(wayfinder, "Wayfinder", "Go, wayfinder/", "EXISTING. Nine-phase planning session. Emits wayfinder.phase.started and wayfinder.phase.completed into a fresh in-process LocalBus each CLI invocation, which resolves to the non-durable wayfinder channel and has no subscriber attached in that process")
    Container_Ext(researchpipeline, "Research Pipeline", "Skill, research-pipeline/", "EXISTING. Five-stage cross-model pipeline: ingest, research, verify+plan, decompose, execute. Human-gated between stages 3 and 4; Stage 4 decomposes into sized Beads, it does not spawn workers directly")
    ContainerDb_Ext(beadsdb, "Beads", "Dolt DB", "EXISTING. Dependency graph, ready/blocked state; state changes are polled today, not emitted")
    System_Ext(github, "GitHub")

    Rel(github, eventbus, "webhook, writes a JSONL file", "cmd/agm-webhook-receiver, compiled and tested by go build ./... but not installed, deployed, or connected to LocalBus/agm-bus, NOT WIRED")
    Rel(wayfinder, eventbus, "wayfinder.phase.started, wayfinder.phase.completed", "EXISTING emit, but into a short-lived unsubscribed bus, see note above")
    Rel(vroom, eventbus, "vroom.decision.* topic constants defined", "NOT WIRED: zero non-test callers of the Emitter's methods")
    Rel(absencealarm, agmbus, "pulse absent, pulse recovered", "NEW wiring")
    Rel(beadsdb, agmbus, "bead.ready, bead.blocked, bead.closed", "NEW: today this is polled, not emitted, and beads' own store is external to this repo")
    Rel(mergeloop, agmbus, "pr.backpressure, pr.merged", "NEW wiring; the backpressure check already exists in mergeloop, it is just not emitted anywhere")

    Rel(eventbus, router, "in-process event stream", "NOT WIRED today: pkg/trigger/subscriber.go subscribes to two wildcard patterns and one exact topic, but has zero production callers")
    Rel(agmbus, router, "cross-process event stream, once agm-bus gains a subscribe primitive", "NEW")
    Rel(router, workflow, "matched trigger starts a workflow run", "NEW: pkg/workflow is not currently event-triggered")
    Rel(workflow, backpressure, "checks policy before admitting a run", "NEW")
    Rel(workflow, agmbus, "emits result event", "NEW")
    Rel(researchpipeline, workflow, "one instance of a pipeline the engine can host", "NEW hosting relationship")
    Rel(researchpipeline, beadsdb, "Stage 4 decomposes into sized beads", "EXISTING skill behavior")

    Rel(agmbus, operator, "notification path", "EXISTING Discord and Matrix adapters; no desktop-notification adapter was found")
    Rel(operator, router, "approves human-review-gated transitions", "EXISTING, PR review / chat")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## 3. Flow A (simple): ticket to assigned worker, as a sequence

This flow is event-triggered choreography, not a closed feedback loop: nothing in it
measures an outcome and feeds that measurement back into a future triage decision. It
is included as the baseline case Flow B is advanced relative to.

```mermaid
sequenceDiagram
    participant GH as GitHub (ticket)
    participant RT as Trigger Router (new bridge)
    participant TRI as Triage Workflow Run (new)
    participant BD as Beads (existing)
    participant VR as VROOM Meta-Orchestrator (existing)
    participant WK as Worker (existing)

    GH->>RT: ticket.created (NEW: routed through the trigger router instead of a person filing a bead by hand)
    RT->>TRI: match "ticket.created", start triage run
    TRI->>TRI: transform: classify severity, assign priority
    TRI->>BD: bd create (bead, priority, acceptance criteria)
    Note over BD,VR: EXISTING and unchanged. VROOM's Meta-Orchestrator reads "bd ready" on its own 180 second tick (ADR-002, "Beads are the roadmap"). This flow does not ask VROOM to subscribe instead of poll.
    BD-->>VR: bead becomes ready (dependency graph resolves)
    VR->>WK: spawn worker (EXISTING AGM path)
    WK->>GH: opens PR
    Note over WK,GH: Mergeloop's own 10 minute tick discovers the new PR (EXISTING, ADR-029). No new event is needed here. Emitting worker.pr_opened would carry no information mergeloop's tick does not already get on its own next pass, so this flow does not add one.
```

## 4. Flow B (advanced): arXiv paper to a merge decision

Like Flow A, the plumbing shown here is event-triggered choreography. The one place a
loop actually closes in this design is separate from both flows: mergeloop's
backpressure signal (`pr.backpressure`) becoming visible enough that a future intake
decision can react to it, and the absence-alarm's alarm-then-recovery cycle, which is
already a closed loop today. See [feedback-loop-pipelines.md, "What makes a loop a
feedback loop"](feedback-loop-pipelines.md#what-makes-a-loop-a-feedback-loop) for that
distinction. Flow B routes through beads the same way Flow A does, so VROOM's ordinary
dispatch capacity, not a bespoke spawn-time check, is what keeps N candidate PRs from
all appearing on the queue at once.

```mermaid
sequenceDiagram
    participant FEED as arXiv/RSS feed
    participant RT as Trigger Router (new bridge)
    participant RP as Research Pipeline (existing skill, hosted as a workflow run)
    participant S4 as Independent Stage 4 Reviewer (existing skill rule, a different model again)
    participant HUM as Operator
    participant BD as Beads (existing)
    participant VR as VROOM (existing, same dispatch path as Flow A)
    participant WK as Candidate Worker (existing AGM path)
    participant BM as Benchmark Runner (new workflow run)
    participant BP as Backpressure Policy (new)
    participant GATE as Merge Decision (illustrative only, needs its own design review)
    participant GH as GitHub

    FEED->>RT: paper.posted
    RT->>RP: match "paper.posted", start research-pipeline run
    RP->>RP: Stage 2, goal-oriented research
    RP->>RP: Stage 3, independent model verifies claims, writes a codebase-grounded plan
    RP->>HUM: present plan (EXISTING human review gate, Stage 3 to Stage 4 boundary)
    HUM-->>RP: approval receipt bound to the exact plan revision, commit SHA or content digest, per the skill's own rule
    RP->>S4: Stage 4, a third independent model adversarially reviews the approved plan before any bead exists
    alt Stage 4 finds a blocking defect
        S4-->>RP: back to Stage 3's author to fix, then re-review, with a fresh approval receipt on the corrected revision
    else Stage 4 ships unconditionally
        S4->>BD: decompose into N sized beads, one per candidate approach, per the skill's actual Stage 4 output
        Note over BD,VR: Same mechanism as Flow A. VROOM dispatches candidate beads from its own ready queue as capacity allows, so N candidates do not all spawn workers simultaneously.
        BD-->>VR: candidate beads become ready
        VR->>WK: spawn worker per bead, as capacity allows (EXISTING AGM path)
        WK->>GH: opens candidate PR
        GH->>RT: benchmark.requested
        RT->>BM: match "benchmark.requested", start benchmark run
        BM->>BP: check backpressure, max concurrent benchmark runs
        alt admitted
            BP-->>BM: proceed
            BM->>RT: benchmark.completed(pr, delta), one independently authored benchmark shared by all N candidates so no candidate marks its own homework
            RT->>GATE: evaluate(delta, change classification)
            Note over GATE: GATE is sketched here to show where the decision plugs in. Its actual rule needs its own design review; see feedback-loop-pipelines.md, "Auto-merge illustration."
            GATE->>GH: request human review, this document does not propose a path that merges without one
        else queued
            BP-->>BM: hold until a slot frees, re-check on the next tick
        else rejected
            BP-->>BM: reject, audit the rejection the same way mergeloop audits its own cap-exceeded ticks
        end
    end
```

Flow B keeps the research-pipeline skill's own Stage 4 decomposition, its
commit-SHA-bound approval receipt, and Flow A's bead-and-VROOM dispatch path intact.
Nothing here proposes skipping any of them. See [feedback-loop-pipelines.md, "Auto-merge
illustration"](feedback-loop-pipelines.md#auto-merge-illustration-not-a-decision) for
why the final gate stays a sketch rather than a shipped rule in this document.
