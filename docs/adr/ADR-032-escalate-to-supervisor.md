<!-- Last audited at: 2026-06-21 -->

# ADR-032: Escalate To Supervisor (VROOM mesh)

**Status:** Accepted
**Date:** 2026-06-18
**Implements:** ADR-031 §3 (the `agm escalate` path it left as future work)
**Bead:** ce-x19g

---

## Context

ADR-031 removed bypass flags and named escalation as the correct response to a
blocked agent, but left `agm escalate` unimplemented. More broadly, an agent in
the mesh that hits any question or decision it cannot resolve had no first-class,
audited way to ask for help, and we had no corpus to mine for "what are agents
repeatedly confused about?" — a direct signal of unclear instructions.

## Decision

A worker escalates a question or decision to whoever spawned it. The chain walks
up the spawn hierarchy:

```
worker → spawning supervisor → … → VROOM trio (confer) → Dispatch → human
```

Each node either answers (if confident) or forwards one hop. Two cases are
decided mechanically, before any routing:

- **Self-evident "proceed with the assigned task"** auto-resolves to "yes" and
  never reaches a human.
- **High-stakes** decisions (product, pricing, strategy, publishing,
  destructive, irreversible, legal, security-policy, spend, people,
  external-comms) are flagged *must-reach-human*: no node below the human may
  give a terminal answer; supervisors may only forward with a recommendation.

### Structure

- **Engine** — `pkg/vroom/escalation` (root module, pure domain logic, no AGM
  imports). Classifier, chain router, store, and the structured log live here
  behind interface seams (`SessionGraph`, `Messenger`, `HumanDispatch`, `Store`,
  `Sink`). Fully unit-tested with in-memory fakes.
- **Adapters + CLI** — `agm/cmd/agm/escalate.go`. `SessionGraph` over the Dolt
  session hierarchy (`GetParent`); `Messenger` over `agm send`; `HumanDispatch`
  over the VROOM Dispatch trail. Exposes `agm escalate ask|answer|forward|list|
  show`.

  The module boundary forces this split: `agm/internal/*` is not importable from
  the root module, so the engine must be pure and the AGM-coupled adapters live
  under `agm/`. This mirrors VROOM PR-1 (pure loops + in-memory substrates).

### Blocking vs async

Only the **asking worker** blocks: `--mode blocking` calls `Raise` then `Await`
(polls the store). Supervisors are **never** blocked — they drain escalations on
their own schedule via `list`/`answer`/`forward`. This is the load-bearing
property: a supervisor managing dozens of agents does not stop its loop for one
question.

### Logging (every call, via OTel + JSONL)

Every state transition emits one `EscalationEvent` (one JSONL line at
`~/.agm/escalation/events.jsonl`) and an OTel span + metrics. The schema is
designed so three analyses fall out by grouping alone:

1. **Incorrect / misaligned answers** — `outcome` field on answered events
   (backfilled by the LLM-judge pass): `WHERE outcome IN ('incorrect',
   'misaligned')`.
2. **Frequent questions / types** — `GROUP BY question_hash | topic | kind`.
3. **Many agents asking the same question** (missing prompt context) —
   `GROUP BY question_hash HAVING COUNT(DISTINCT origin_session_id) > N`.

`question_hash` is a sha256 of the normalised question, so (2) and (3) are
robust to trivial wording differences.

### The adjudicator + analysis CLI (ce-irr0)

The `outcome`/`misalignment` columns are empty at write time and backfilled by
an LLM **adjudicator** that mirrors the `internal/override.Judge` seam:
`Adjudicator` (interface) → `DefaultAdjudicator` (deterministic floor; only the
decidable non-answer case is scored offline) → `ClaudeAdjudicator` (layers a
model classifier on top, a *separate* model from any agent in the chain). It
degrades safely — no `ANTHROPIC_API_KEY` ⇒ the floor; a model error never
invents a verdict, it leaves the event for a later pass. The pass is a rewrite
of the JSONL log (atomic temp+rename), idempotent unless `--force`.

The three analyses are pure functions over the log (`Summarize` folds events to
one record per escalation; `AnalyzeMisaligned` / `AnalyzeFrequentQuestions` /
`AnalyzeManyAgents`), surfaced as:

```
agm escalate adjudicate                 # LLM judge pass → backfill columns
agm escalate analyze misaligned         # analysis (1)
agm escalate analyze frequent  [--min]  # analysis (2)
agm escalate analyze duplicates [--min] # analysis (3): many agents, same question
agm escalate analyze all
```

## Consequences

**Positive.** Agents have an audited ask-up path; product decisions provably
reach the human while task-confirmations never do; the log is a direct
instrument for improving prompts and instructions.

**Trade-offs / follow-ups (filed as beads).**
- "Confer" among the VROOM trio was originally modelled as `PhaseConferring` +
  the trio's own peer messaging; programmatic quorum voting is now implemented
  (ce-es7z, see the addendum below).
- The `outcome`/`misalignment` columns are backfilled by the LLM-judge pass and
  the analysis CLI over the log shipped in ce-irr0 (see "The adjudicator +
  analysis CLI" above). A stronger default model and scheduled/auto adjudication
  remain future work.
- The supervisor loop's `Tick` auto-drains escalations each iteration via the
  `EscalationDrainer` seam (`pkg/vroom/supervisor`, ce-xshb): a programmatic
  supervisor lists the escalations it holds and answers/forwards each per a
  pluggable `DrainPolicy`, rather than relying on a live agent reading the
  protocol skill and running the CLI by hand. The default policy forwards
  everything up the chain (a Go supervisor has no way to author a confident
  answer); an answering policy can be injected. Wiring the seam to a
  FileStore-backed engine in the agent-facing binary (which owns the session
  hierarchy adapters) is the remaining integration step.
- `FileStore` is last-writer-wins on concurrent answers to one escalation
  (acceptable for v1; rare in practice).

The temporal design record (charter → spec → plan) is in engram-research:
`projects/vroom-escalate-to-supervisor/WAYFINDER.md`.

---

## Addendum (ce-es7z): programmatic trio confer

The original `PhaseConferring` delivered an escalation to a *single* VROOM node
(the one the spawn graph walked to), which then conferred with its two peers via
informal peer messaging and answered or forwarded on their behalf. That left the
confer invisible to the audit log and dependent on one supervisor faithfully
polling the other two. ce-es7z replaces it with a **programmatic quorum vote**.

**Registry.** A new `VROOMRegistry` seam yields the live trio. The agm adapter
(`doltVROOMRegistry`) resolves the three well-known names
(`vroom-meta-orchestrator`, `vroom-orchestrator`, `vroom-overseer`) to their
current session IDs; members not currently live are skipped, and if none resolve
the engine degrades gracefully to the original single-node conferring.

**Fan-out + vote.** When an escalation reaches the mesh with a registry
configured, the engine sets `Escalation.Confer` (members + quorum + ballots),
sets the holder to the sentinel `vroom-trio`, and delivers the question to *all*
members at once. Each member votes via `agm escalate vote <id> approve|reject|
abstain` (engine: `Engine.CastVote`). Resolution:

- a **quorum of matching approvals** (2 of 3 by default) answers the escalation
  and returns the answer to the asking worker;
- a **quorum of rejections**, or **every member voting** without an answer
  quorum (deadlock), dispatches to the human via Dispatch;
- a **must-reach-human** escalation is never answered by the trio — the vote only
  gathers the trio's recommendation, then dispatches to the human.

Each member votes once; only invited members may vote; while a confer is in
flight a member must vote rather than `answer` unilaterally (the human may still
`answer --by human` to override). Every ballot emits an `EscalationEvent` with a
new `vote` field, so trio voting patterns are queryable alongside the existing
analysis keys.

**Boundary preserved.** The engine stays pure (`pkg/vroom/escalation`); the
registry and the `vote` CLI are AGM adapters (`agm/cmd/agm/escalate.go`).
