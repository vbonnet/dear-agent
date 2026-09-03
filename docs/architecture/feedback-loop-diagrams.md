# Feedback-Loop Architecture: Diagrams

<!-- Last audited at: 2026-09-02 -->

Companion diagrams for [feedback-loop-pipelines.md](feedback-loop-pipelines.md). Two
levels use the C4 model (System Context, Container); the two example flows are
expressed as sequence diagrams instead of C4 Component diagrams, because a flow is a
temporal chain of events across several containers, not the internal structure of one
container. All diagrams are Mermaid, in fenced code blocks, and render natively on
GitHub and in Claude Artifacts.

Every `Rel()` and message below is labeled EXISTING or NEW. An EXISTING label means
the wiring runs in a production binary today, not merely that the code exists
somewhere in the tree; see [feedback-loop-pipelines.md, "What already
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

    Rel(github, feedback, "ticket created, PR opened/updated", "webhook + poll")
    Rel(feeds, feedback, "new paper posted", "RSS poll")
    Rel(cron, feedback, "scheduled tick", "launchd")
    Rel(agents, feedback, "raises problem, proposes improvement", "escalation, decision trail")
    Rel(beads, feedback, "bead ready, blocked, closed", "state change; NEW as an emitted event, today it is polled")
    Rel(feedback, github, "opens PR, comments, merges", "safe-* / GitHub API")
    Rel(feedback, beads, "creates and updates beads", "bd CLI")
    Rel(feedback, operator, "notifies, requests review", "desktop notification, PR review request")
    Rel(operator, feedback, "approves gate, answers escalation", "PR review, chat")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## 2. Container

Inside the system boundary. The cross-process transport question matters here:
`pkg/eventbus.LocalBus` is documented as "an in-process event bus implementation"
(`pkg/eventbus/local_bus.go:17`); it does not cross a process boundary on its own.
Every trigger in this document's example flows originates in a different process from
the one that should react to it (mergeloop and absence-alarm are separate launchd
one-shots; VROOM supervisors are separate harness sessions). `agm-bus`
(`agm/internal/bus`, `agm/cmd/agm-bus`) already is the cross-process transport: a
durable unix-socket broker with a per-session offline queue, already designed to carry
"side-channel observability events from infrastructure code... that originate outside
any running Claude session" (`agm/internal/bus/emitter.go`). This design routes
cross-process trigger events over `agm-bus`, and keeps `pkg/eventbus` for fan-out
inside one process.

```mermaid
C4Container
    title Container: Event/Trigger/Pipeline System

    Person(operator, "Operator")

    System_Boundary(feedback, "Event/Trigger/Pipeline System") {
        Container_Ext(agmbus, "agm-bus", "Go, agm/internal/bus + agm/cmd/agm-bus", "EXISTING. Cross-process transport: unix socket, durable per-session offline queue, ACL. Already carries infrastructure-origin events")
        Container(router, "Trigger Router", "Go, pkg/trigger, extended", "EXISTING as an engram-only matcher wired to pkg/eventbus (pkg/trigger/subscriber.go); extend to wildcard matching and a registry-driven subscription set, and to publish over agm-bus for cross-process delivery")
        Container_Ext(workflow, "Workflow Engine", "Go, pkg/workflow", "EXISTING (ADR-010). YAML dependency-ordered runs, SQLite state, per-transition audit, first-class HITL. Proposed as the pipeline runtime, extended to be startable from a trigger match instead of only a CLI invocation")
        Container(backpressure, "Backpressure Policy", "Go, new pkg/pipeline/backpressure", "NEW. Generalizes mergeloop's cap-and-skip into a reusable cap(N)/timeout(D)/staleness(D) policy any workflow run or trigger match can declare")
    }

    Container_Ext(eventbus, "Event Bus (in-process)", "Go, pkg/eventbus", "EXISTING. Four channels, typed Event, Subscribe/Emit/AddSink. Fans out within one running process; does not itself cross a process boundary")
    Container_Ext(absencealarm, "Absence Alarm", "Go, cmd/absence-alarm", "UNMERGED (stack/absence-alarm-*, now committed on that branch but not yet in main). Pulse registry, scheduler, alarm sink. Alarms when an expected periodic signal (file mtime, launchd job presence, command exit) goes stale; the probes themselves are polled, not event-driven")
    Container_Ext(vroom, "VROOM Supervisors", "Go, cmd/vroom-dispatch", "EXISTING. Three supervisor harness sessions on independent tick timers (Meta-Orchestrator 180s, Orchestrator 90s, Overseer 60s), driven by /loop prompts, not an event loop")
    Container_Ext(mergeloop, "Mergeloop", "Go, cmd/mergeloop", "EXISTING. 10-minute launchd tick, cap-based backpressure, audits but does not emit its findings")
    Container_Ext(wayfinder, "Wayfinder", "Go, wayfinder/", "EXISTING. Nine-phase planning session. Emits wayfinder.phase.started and wayfinder.phase.completed to pkg/eventbus today")
    Container_Ext(researchpipeline, "Research Pipeline", "Skill, research-pipeline/", "EXISTING. Five-stage cross-model pipeline: ingest, research, verify+plan, decompose, execute. Human-gated between stages 3 and 4")
    ContainerDb_Ext(beadsdb, "Beads", "Dolt DB", "EXISTING. Dependency graph, ready/blocked state; state changes are polled today, not emitted")
    System_Ext(github, "GitHub")

    Rel(github, eventbus, "webhook, one-off spike scaffolding, writes a JSONL file today", "cmd/agm-webhook-receiver, NOT wired to pkg/eventbus")
    Rel(wayfinder, eventbus, "wayfinder.phase.started, wayfinder.phase.completed", "EXISTING, wayfinder/internal/analytics/session_tracker.go")
    Rel(vroom, eventbus, "vroom.decision.* topic constants defined, no production emitter wired", "NOT EXISTING despite the topic constants existing")
    Rel(absencealarm, agmbus, "pulse absent, pulse recovered", "NEW wiring")
    Rel(beadsdb, agmbus, "bead.ready, bead.blocked, bead.closed", "NEW: today this is polled, not emitted, and beads' own store is external to this repo")
    Rel(mergeloop, agmbus, "pr.backpressure, pr.merged", "NEW wiring; the backpressure check already exists in mergeloop, it is just not emitted anywhere")

    Rel(eventbus, router, "in-process event stream", "EXISTING wiring, pkg/trigger/subscriber.go, currently three hardcoded topics")
    Rel(agmbus, router, "cross-process event stream", "NEW wiring")
    Rel(router, workflow, "matched trigger starts a workflow run", "NEW: pkg/workflow is not currently event-triggered")
    Rel(workflow, backpressure, "checks policy before admitting a run")
    Rel(workflow, agmbus, "emits result event", "NEW")
    Rel(researchpipeline, workflow, "one instance of a pipeline the engine can host")

    Rel(agmbus, operator, "notification path", "EXISTING mechanism exists on agm-bus; confirm which sink carries a desktop notification before relying on this")
    Rel(operator, router, "approves human-review-gated transitions")

    UpdateLayoutConfig($c4ShapeInRow="3", $c4BoundaryInRow="1")
```

## 3. Flow A (simple): ticket to assigned worker, as a sequence

```mermaid
sequenceDiagram
    participant GH as GitHub (ticket)
    participant RT as Trigger Router (extended)
    participant TRI as Triage Workflow Run (new)
    participant BD as Beads (existing)
    participant VR as VROOM Meta-Orchestrator (existing)
    participant WK as Worker (existing)

    GH->>RT: ticket.created (NEW: routed through the trigger router instead of a person filing a bead by hand)
    RT->>TRI: match "ticket.created", start triage run
    TRI->>TRI: transform: classify severity, assign priority
    TRI->>BD: bd create (bead, priority, acceptance criteria)
    Note over BD,VR: EXISTING and unchanged: VROOM's Meta-Orchestrator reads "bd ready" on its own 180s tick (ADR-002, "Beads are the roadmap"). This flow does not ask VROOM to subscribe instead of poll.
    BD-->>VR: bead becomes ready (dependency graph resolves)
    VR->>WK: spawn worker (EXISTING AGM path)
    WK->>GH: opens PR
    Note over WK,GH: mergeloop's own 10-minute tick discovers the new PR (EXISTING, ADR-029). No new event is needed here: emitting worker.pr_opened would carry no information mergeloop's tick does not already get on its own next pass, so this flow does not add one.
```

## 4. Flow B (advanced): arXiv paper to auto-merge or human review

```mermaid
sequenceDiagram
    participant FEED as arXiv/RSS feed
    participant RT as Trigger Router (extended)
    participant RP as Research Pipeline (existing skill, hosted as a workflow run)
    participant S4 as Independent Stage 4 Reviewer (existing skill rule, a different model again)
    participant HUM as Operator
    participant EXP as Experimental Workers (existing AGM spawn)
    participant BM as Benchmark Runner (new workflow run)
    participant BP as Backpressure Policy (new)
    participant GATE as Merge Decision (illustrative only, needs its own design review)
    participant GH as GitHub

    FEED->>RT: paper.posted
    RT->>RP: match "paper.posted", start research-pipeline run
    RP->>RP: Stage 2: goal-oriented research
    RP->>RP: Stage 3: independent model verifies claims, writes a codebase-grounded plan
    RP->>HUM: present plan (EXISTING human review gate, Stage 3 to Stage 4 boundary)
    HUM-->>RP: approval receipt bound to the exact plan revision (commit SHA or content digest, per the skill's own rule)
    RP->>S4: Stage 4: a third independent model adversarially reviews the approved plan before any bead exists
    alt Stage 4 finds a blocking defect
        S4-->>RP: back to Stage 3's author to fix, then re-review, with a fresh approval receipt on the corrected revision
    else Stage 4 ships unconditionally
        S4->>RT: plan.approved
        RT->>EXP: spawn N competing experimental workers, one per candidate approach
        EXP->>BM: each worker's PR triggers benchmark.requested
        BM->>BP: check backpressure (max concurrent benchmark runs, PR-queue cap)
        BP-->>BM: admit, queue, or reject
        BM->>RT: benchmark.completed(pr, delta), one independently authored benchmark shared by all N candidates so no candidate marks its own homework
        RT->>GATE: evaluate(delta, change classification)
        Note over GATE: GATE is sketched here to show where the decision plugs in. Its actual rule needs its own design review; see feedback-loop-pipelines.md, "Auto-merge illustration."
        GATE->>GH: request human review (this document does not propose an auto-merge path that ships without that review)
    end
```

Flow B keeps the research-pipeline skill's own Stage 4 adversarial review and its
commit-SHA-bound approval receipt intact; nothing here proposes skipping either. See
[feedback-loop-pipelines.md, "Auto-merge
illustration"](feedback-loop-pipelines.md#auto-merge-illustration-not-a-decision) for
why the final gate stays a sketch rather than a shipped rule in this document.
