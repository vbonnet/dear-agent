# ADR-023: Agent Friction Reporting and Session Handoff

**Status**: Proposed
**Date**: 2026-05-21
**Context**: Two AGM capabilities surfaced by external practice — Lovable's
`/vent` self-report tool and Matt Pocock's `/handoff` skill — examined for
dear-agent fit. This is a **design** ADR: it fixes vocabulary, command
surface, and integration seams so that a later implementation ADR (or a DEAR
Define artifact) can proceed without re-litigating naming or shape. No code
ships with this ADR.

Builds on / aligns with:

- [ADR-007 (agm): Hook-Based State Detection](../../agm/docs/adr/ADR-007-hook-based-state-detection.md)
  — the DONE/WORKING/COMPACTING/USER_PROMPT/OFFLINE state machine the
  `is_stuck` classifier (§F6) derives from.
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md) — friction
  reports are an Audit-phase finding stream; the triage agent (§F4) is an
  Audit consumer.
- [ADR-018: Graceful Exit as a Framework Default](ADR-018-graceful-exit-framework-default.md)
  — the session-start banner pattern reused for the handoff staleness warning
  (§H6).
- [ADR-022: Backlog Suggestion System](ADR-022-backlog-suggestion-system.md) —
  clustered friction becomes a new `backlog.Source` (§F4).
- [ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md)
  and [/CONTEXT.md](../../CONTEXT.md) — the decision-trail / supervisory-mesh
  vocabulary both features emit into.
- [ADR-012: Provider Transport Layer](ADR-012-provider-transport-layer.md) and
  agm [ADR-011: Gemini CLI Adapter](../../agm/docs/adr/ADR-011-gemini-cli-adapter-strategy.md)
  — the adapter seam that makes cross-model handoff (§H5) possible.

---

## Context

Two recurring gaps in dear-agent's own operation, each with a credible
external precedent:

**1. Friction is invisible until the Retro — if it survives that long.**
When a worker hits an impediment mid-task (a permission denial, a flaky test,
an ambiguous spec, a missing capability, a tool that fights it), the friction
is felt in the **Execute** phase but recorded — if at all — only at **Retro**,
reconstructed from memory days later. Most friction never reaches a retro: the
worker works around it, the session ends, the data point is lost. The
`docs/retros/` history shows this directly — retros are written when a *human*
notices a pattern (CI red streaks, doc drift), not when an agent hits a wall.
Lovable's `/vent` tool closes this gap by letting the agent self-report
friction in the moment, routed to a channel where a triage agent acts on it.
We want the same real-time channel, but the name "vent" is wrong for us
(§F1) and the destination should be our own surfaces, not Slack (§F3).

**2. We have lossy in-place compaction but no scoped fork.** AGM already ships
two compaction commands:

- `agm send compact <session>` — low-level: sends `/compact` into a running
  tmux session with anti-loop safety (2h cooldown, max 3/lifetime), an
  auto-generated PRESERVE prompt, and an audit trail in
  `~/.agm/compaction-prompts/`.
- `agm session compact <id>` — a higher-level wrapper adding Dolt session
  resolution, pre-flight state detection, and completion monitoring.

Both operate on the **same** session, **in place**, and are **lossy**: they
shrink the context window of an ongoing conversation by summarizing it. What
neither does is **extract a scoped slice of work into a fresh session**. Matt
Pocock's `/handoff` skill names exactly that missing operation: rather than
compress one session, lift the part that matters into a new one with a clean
budget. The risk — flagged explicitly in the task brief — is shipping a third
"compact-ish" command that blurs into the two we have. This ADR's job is to
make the distinction sharp enough that it cannot blur (§H1).

---

## Decision (Part A): Friction Reporting

### F1. Name: `agm friction` (a "friction report"), not `vent`

The Lovable name "vent" frames the act as emotional discharge — the agent
*complains*. dear-agent's value is that a report is a **constructive,
blameless observation of a system impediment**: "the substrate has friction
here," not "I am frustrated." The name must carry "I'm reporting an
impediment," and it must not collide with vocabulary we already own. Four
candidates were considered:

| Name | Verb-noun | Rationale | Verdict |
|------|-----------|-----------|---------|
| **`friction`** | "log friction" / "a friction report" | Names a *system property*, not an emotion. Mechanical and blameless — friction is a fact about the surface, not a feeling about it. Matches the industry-standard "friction log" term, so it imports a known practice. | **Recommended** |
| `snag` | "hit a snag" | Concrete, blameless, action-oriented; a snag is plainly an impediment. | Slightly informal; weaker tie to existing practice. Good fallback. |
| `surface` | "surface an issue" | Emphasizes the real point — making buried friction *visible*. Constructive framing. | Transitive verb needs an object; reads awkwardly as a bare command (`agm surface`?). |
| `note` | "note an impediment" | Lowest emotional charge; purely observational. | Too soft — loses the "this is blocking me" urgency — and collides with general note-taking. |

Two names were **rejected outright**:

- **`signal`** — already owned. ADR-015's signal-aggregator defines `Signal`
  as a project-health metric (commits/week, lint count), and the VROOM
  decision trail uses "signal" loosely. Reusing it for friction would muddy a
  type that ADR-022 deliberately kept separate from task-driven inputs. This
  is exactly the "Known Terminology Collisions" hazard CONTEXT.md warns about.
- **`flag`** — carries blame and escalation connotations ("flag *someone*").
  Friction reporting must stay blameless.

Decision: the command is **`agm friction`**; the artifact it produces is a
**friction report**.

### F2. Where it sits in the DEAR loop

Friction reporting is the **real-time Execute→Audit→Retro bridge** that DEAR
currently lacks. Today the loop is:

```
Define → Execute → Audit → Retro
                            ↑ friction reconstructed from memory here (lossy)
```

With `agm friction` the worker emits the impediment at the moment of contact,
during Execute. The report lands in the Audit finding stream (ADR-011) and is
carried, already structured, into Retro:

```
Define → Execute ──(agm friction)──> Audit finding stream ──> Retro (data-driven)
            │                              │
            └── continues working ─────────┘  (reporting is non-blocking)
```

The key property: **reporting does not interrupt the work.** `agm friction`
records and returns; the worker keeps going (or escalates if `--block` is
set and the impediment is genuinely fatal). This mirrors the graceful-exit
philosophy (ADR-018): the right behavior is made cheap and in-band, not
bolted on as a separate ritual.

### F3. Destination: structured local ledger + VROOM event, not Slack

Lovable routes `/vent` to Slack. dear-agent's equivalent surfaces are its own:

1. **Append-only JSONL ledger** at `~/.agm/friction/<date>.jsonl`, mirroring
   the established `~/.agm/compaction-prompts/` audit-trail pattern. This is
   the durable, machine-readable record of record.
2. **A VROOM decision-trail event** — new topic `vroom.friction.reported` —
   so the Overseer (CRO) and Meta-Orchestrator (CTO) see friction without
   tailing a file, and it joins the append-only governance log (ADR-002,
   CONTEXT.md). Emission is synchronous (the ADR-022 §D5 lesson: a short-lived
   CLI must not rely on a backgrounded `Emitter` goroutine that `os.Exit`
   races).

We deliberately do **not** write friction reports into beads (`bd`) or
`docs/retros/` directly. Per the WorkItem-ledger constraint, agent `bd` writes
require explicit in-conversation instruction; the triage agent (§F4) *proposes*
backlog/retro items for human approval rather than committing them. And we do
**not** add a Slack/Asana hop: that would re-route dear-agent's signal out of
the substrate that already governs it.

### F4. A triage agent — and how it connects to the backlog ranker

Yes, there should be a triage step, but it is a **batch consumer**, not a
synchronous responder. A `friction-triage` agent (an Audit-phase / VROOM
Orchestrator-adjacent role) runs on a cadence (daily, or on a threshold count)
and:

1. Reads the JSONL ledger since the last run.
2. Clusters reports by `category` + `repo` + similarity (repeated identical
   permission denials are one cluster, not twenty findings).
3. For each cluster above a salience threshold, **proposes** one of:
   a backlog item, a retro, an ADR, or a one-line `.dear-agent.yml` /
   permission-rule fix — as a *suggestion*, surfaced for human approval.

This plugs directly into ADR-022: a clustered friction theme becomes a new
`backlog.Source` implementation, so friction-derived work flows through the
*same* Orchestrator dispatch ranker as everything else, with a
`vroom.decision.dispatched` row when acted on. Friction does not get a
privileged side-channel; it earns its place in the backlog like any item.

### F5. Friction report metadata

A report is structured so it is clusterable and auditable without NLP:

```jsonc
{
  "id": "fr-<ulid>",
  "timestamp": "2026-05-21T15:02:36Z",
  "session_id": "<uuid>",
  "harness": "claude-code",          // claude-code | gemini-cli | codex-cli | ...
  "model": "claude-opus-4-7",
  "repo": "dear-agent",
  "git_ref": "<sha>",                // commit at time of report
  "dear_phase": "execute",           // define | execute | audit | retro
  "task_id": "6.3",                  // backlog/acceptance id if known
  "category": "permission|tooling|docs|flaky-test|missing-capability|spec-ambiguity|environment",
  "severity": "low|medium|high",     // impact on the current task
  "is_stuck": false,                 // set true if the is_stuck classifier (§F6) co-fired
  "description": "free text — what was hit",
  "evidence": "command + error excerpt (verbatim, truncated)",
  "suggested_fix": "optional — what would have unblocked me"
}
```

`category` is a closed enum on purpose: it is what the triage clusterer keys
on. `evidence` is verbatim (no agent paraphrase) so the cluster is groundable.
`suggested_fix` is optional — a report is valid with nothing but a category
and a description (the graceful-exit principle: an honest "I'm blocked and I
don't know the fix" is a first-class report).

### F6. Relationship to the `is_stuck` classifier

`is_stuck` (Lovable) and `agm friction` are **complementary and orthogonal**:

|              | `is_stuck` | `agm friction` |
|--------------|------------|----------------|
| Trigger      | Passive — inferred from telemetry (repeated tool failures, edit/revert loops, no diff progress, state thrash) | Active — the agent voluntarily reports |
| Source       | The ADR-007 hook state machine, extended with a derived "stuck" state | The worker's own judgment |
| Failure mode | False positives (looks stuck, isn't) | Silence (is stuck, never says so) |

They cover each other's blind spot. `is_stuck` firing **without** a
corresponding friction report is itself a high-value signal — the agent is
struggling and not self-aware enough to report it — and should auto-prompt:
"you appear stuck; consider `agm friction`." Conversely, a friction report
tagged `is_stuck: true` is a priority cluster for triage. `is_stuck` is on the
roadmap (§R4) as a behavioral classifier built on ADR-007; this ADR only fixes
the seam (`is_stuck` co-fires into the friction report's boolean field).

---

## Decision (Part B): Session Handoff

### H1. Handoff is `agm session new --from`, NOT a new command

A handoff **produces a new session**. That is its entire mechanical identity.
The only thing distinguishing a handoff from a blank `agm session new` is the
*seed content* of the new session. Therefore handoff is **not** a new
top-level command — it is two flags on session creation:

```
agm session new --from <session-id> --scope "<description>" [--harness ...] [--model ...]
```

The reasoning is decisive and matches the task brief's own framing:

- A separate `agm handoff` command would have to **duplicate the entire
  `agm new` flag surface** — `--harness`, `--model`, `--workspace`,
  `--sandbox`, `--role`, `--tags`, `--permission-profile`,
  `--inherit-permissions`, `--workflow`. That is the "two similar commands"
  anti-pattern Valentin warned against, in its purest form.
- Composing onto `new` means a handoff inherits *all* of creation's behavior
  for free: a handoff can target a different harness (`--harness gemini-cli`,
  enabling §H5), reuse a permission profile, land in a sandbox, etc.
- `--from` is the seed source; `--scope` is the slice selector. Everything
  else is "how to make a session," which `new` already owns.

This keeps AGM's command surface honest: **compaction is a verb on an existing
session; handoff is an adjective on a new one.**

### H2. The crisp distinction (this table is the load-bearing artifact)

|                      | `agm send/session compact` | `agm session new --from --scope` |
|----------------------|----------------------------|----------------------------------|
| Operates on          | the **same** session        | a **new** session                |
| Effect on context    | shrinks in place (lossy)    | starts fresh with a scoped seed  |
| What survives        | a summary of *everything*   | only the slice matching `--scope`|
| Token budget         | continues (reduced)         | resets (fresh budget)            |
| Intent               | "keep going, smaller"       | "carve off a sub-problem / hand to another worker/model" |
| Continuity boundary  | none — one continuous thread| explicit — a deliberate cut      |
| Chain of custody     | n/a (same session)          | `parent_session_id` link (§H4)   |
| Cross-harness        | no (in-harness `/compact`)  | yes (manifest is harness-agnostic, §H5) |

If a proposed feature does not move a row in this table, it is not a new
feature — it is one of the two we already have.

### H3. Unit of transfer: a structured handoff manifest, not prose

The thing that moves is a **handoff manifest** — structured frontmatter plus a
markdown body — not a free-text blob. Prose alone is what makes handoffs
silently rot (§H6). Shape:

```yaml
---
handoff_version: 1
parent_session_id: <uuid>
parent_last_decision_event: <vroom-event-id>   # links the audit chain (§H4)
scope: "implement the retry/backoff for the gemini adapter"
captured_at_ref: <sha>                          # git HEAD at handoff time (§H6)
acceptance_criteria:                            # carried from pkg/acceptance
  - type: tests-pass
  - type: lint-clean
files_in_play:
  - agm/internal/agent/gemini/transport.go
decisions_so_far:
  - "chose exponential backoff capped at 30s (parent session)"
open_questions:
  - "should 429 be retried or surfaced?"
out_of_scope:                                   # explicit — prevents scope creep
  - "the broader provider-transport refactor (ADR-012)"
---

<markdown body: the human-readable narrative the worker reads first>
```

The frontmatter is machine-auditable (staleness check, chain walk, acceptance
inheritance); the body is what the receiving agent reads. The **extraction
engine reuses what exists**: `compaction.GeneratePreservePrompt` already knows
how to distill a session's state into a PRESERVE prompt — handoff runs the same
distillation but *scoped* to `--scope` (filter to the slice) and *typed* into
the manifest, rather than dumped as one summary. `acceptance_criteria` reuses
the typed `pkg/acceptance` model (ADR-018 §D1 precedent), so the child session
inherits a real Define contract, not a vibe.

### H4. Chain of custody: reuse `parent_session_id`, add a handoff event

AGM **already has** the substrate for this — `parent_session_id` and
`agm admin link-session-parent` (with `<parent>-exec` naming continuity). A
handoff:

1. Creates the child session via the normal `new` path.
2. Sets `parent_session_id = <--from>` (the existing column; no new schema).
3. Emits `vroom.handoff.created` to the decision trail, referencing the
   parent's `parent_last_decision_event` so the chain is linked end to end.
4. Persists the manifest to `~/.agm/handoffs/<child-session-id>.md` (audit
   trail, same pattern as compaction prompts).

The chain is then queryable by walking `parent_session_id` (an
`agm session get --ancestry`-style view), and every hop has a decision-trail
event and a persisted manifest. Chain of custody is thus **append-only and
reconstructable**, satisfying the dogfooding/audit mandate without inventing
new storage.

### H5. Cross-model handoff (Claude → Codex → Gemini)

This is the payoff of choosing a structured, harness-agnostic manifest over an
in-harness `/compact`. Because the manifest is YAML frontmatter + markdown — not
a Claude-specific compaction transcript — a handoff can target any harness:

```
agm session new --from <claude-session> --scope "..." --harness gemini-cli
```

The manifest is **re-rendered into the target harness's prompt format** by the
existing provider/transport adapter layer (ADR-012) and the per-harness
adapters (agm ADR-011 Gemini, the Codex/OpenCode adapters). `/compact` cannot
do this — it is in-harness and non-portable. The handoff manifest is the
**lingua franca** between models: Claude distills it, the adapter renders it,
Gemini or Codex picks it up. This makes "council of agents" / cross-model
review (§R1) mechanically cheap — each reviewer gets a scoped handoff, not a
copy-pasted dump.

### H6. Failure mode: the stale handoff manifest

The dominant failure mode — and one this repo has been bitten by (stale-base
merges, merge-tree false-safes) — is a handoff doc that **confidently asserts
state that is no longer true**: it names a file that moved, a decision that was
reverted, a ref that the branch has advanced past. The mitigation is to make
staleness **detectable, never silent**:

- The manifest records `captured_at_ref` (the git SHA at handoff time).
- On the child session's **first action**, it diffs `captured_at_ref` against
  current `HEAD` and against the `files_in_play` list. If either has moved, it
  prints a **staleness banner** at session start — the exact ADR-018 pattern
  ("the worker was told this; here is what changed since"). The worker is not
  blocked, but it is *informed* that the manifest may be stale and which parts.
- `out_of_scope` and `open_questions` are first-class fields precisely so the
  receiving agent treats the manifest as a *briefing*, not gospel — the
  graceful-exit / handoff-confidence lesson from ADR-018 §Context.

We do **not** try to keep manifests live-synced with the parent; a handoff is a
deliberate **cut**, and a cut that silently re-syncs is just a slow compaction.
The discipline is: capture the ref, check the ref, surface the drift.

### H7. Relationship to VROOM supervisor handoffs

These are **two layers, not two implementations of one thing**:

- **VROOM role transitions** among the three supervisors (Orchestrator →
  Overseer → Meta-Orchestrator) — and the per-task Primary / Secondary /
  Tertiary responsibilities beneath them — are handoffs of *authority and
  decision* within a mission, the governance layer (ADR-002, CONTEXT.md). (Note:
  there is no standing "Verifier" or "Requester" *role*; verification is the
  Secondary's responsibility. The old five-role expansion is superseded — see
  CONTEXT.md § Status note.)
- **Session handoff** (`--from`) is the *execution-substrate* mechanism — the
  plumbing.

A VROOM role transition can **ride on** a session handoff: when the
Orchestrator (COO) dispatches a unit of work, it does so by creating a worker
session `--from` the mission session, and the manifest's
`parent_last_decision_event` links into the VROOM trail. But not every session
handoff is a VROOM transition (a worker can hand a sub-problem to a fresh
session without changing roles), and not every VROOM transition needs a new
session (the Secondary may verify in place). Keeping the layers distinct
prevents the conflation CONTEXT.md already had to untangle once.

---

## Consequences

### Positive

- DEAR gains a real-time Execute→Audit channel; retros become data-driven
  (clustered friction) instead of memory-driven.
- Friction routes through dear-agent's own surfaces (ledger + VROOM trail +
  backlog ranker), dogfooding the mesh instead of exporting to Slack.
- The compact/handoff distinction is fixed in one table (§H2); a future
  reviewer can reject a blurry "compact-ish" proposal by pointing at it.
- Handoff reuses three existing substrates (`parent_session_id`,
  `compaction.GeneratePreservePrompt`, `pkg/acceptance`) — small surface area.
- The harness-agnostic manifest makes cross-model handoff and council-of-agents
  review mechanically cheap (§H5, §R1).
- Staleness is detectable by construction (§H6), addressing the repo's
  recurring stale-base / false-safe failure class.

### Negative

- `agm friction` adds an agent-facing verb workers must learn to reach for; if
  they don't, the channel stays empty. Mitigation: the `is_stuck` co-prompt
  (§F6) nudges reporting at the moment it matters.
- The friction ledger can accumulate noise; the triage clusterer (§F4) and the
  closed `category` enum (§F5) are the defense, but a poorly-tuned salience
  threshold could either spam or swallow the backlog.
- `--from` adds branching to the already-large `agm new` code path (2000+
  lines). The manifest extraction must degrade gracefully if the parent
  session's state is unavailable (fall back to a prose-only manifest with a
  loud warning).
- A structured manifest is more work to produce than a `/compact` summary; the
  cost is justified only because it buys staleness-checking and portability.

### Neutral

- No schema change for handoff (reuses `parent_session_id`); the friction
  ledger is a new but self-contained JSONL store under `~/.agm/`.
- `is_stuck` is named here but deferred to its own roadmap item (§R4); this ADR
  only reserves the metadata seam.
- Both features emit new VROOM topics (`vroom.friction.reported`,
  `vroom.handoff.created`); these are additive to the decision trail.

---

## Roadmap (from external research, May 2026)

This ADR formalizes two of a larger set of items surfaced by reviewing
external agent-engineering practice. Recorded here so they are tracked; each
non-trivial item will get its own Define artifact / ADR before implementation.

| # | Item | Provenance | Lands in / builds on |
|---|------|------------|----------------------|
| R1 | **Council-of-agents / multi-model review** — formalize the multi-model review pattern (each model reviews independently, results merged). | Delivery Hero | *Validates the existing VROOM pattern* — verification as the Secondary's responsibility (agm ADR-021 is the superseded "Verifier role" stub) + cross-model handoff (§H5). Formalization, not new architecture. |
| R2 | **Agent friction reporting** (`agm friction`). | Lovable `/vent`, reframed | **This ADR, Part A.** |
| R3 | **Session handoff** (`agm session new --from --scope`). | Matt Pocock `/handoff` | **This ADR, Part B.** |
| R4 | **`is_stuck` behavioral classifier** — derive a "stuck" state from telemetry (failure loops, no-progress, state thrash). | Lovable | Extends agm ADR-007 hook state machine; co-fires into friction reports (§F6). |
| R5 | **Knowledge decay / success-ratio pruning for engram** — age out or down-rank memories that stop earning retrievals; prune by success ratio. | Lovable | Builds on `pkg/engram` [ADR-003 memory-strength-tracking-fields](../../pkg/engram/docs/adrs/ADR-003-memory-strength-tracking-fields.md). Decay is the missing *write-down* half of strength tracking. |
| R6 | **Progressive trust ladder for agent autonomy** — graduated permission tiers an agent earns/loses based on track record. | Anthropic workshop | Builds on existing `--permission-profile` (worker/monitor/audit), `--mode` (plan/auto/default), and `rbac.ResolvePermissions`. The ladder is policy over primitives we already have. |
| R7 | **"Context is the ceiling" — token-budget tracking** — treat the context window as the binding constraint; track and surface per-session token usage. | Anthropic workshop | Builds on `agm session context` and `agm new --max-budget-usd`; informs *when* to compact (Part B) vs hand off. |

---

## Implementation (deferred — design only)

This ADR ships no code. A subsequent implementation ADR / DEAR Define artifact
would cover, at minimum:

- `agm friction` command + `~/.agm/friction/<date>.jsonl` ledger +
  `vroom.friction.reported` topic.
- `friction-triage` agent + a `backlog.Source` adapter (ADR-022 seam).
- `agm session new --from --scope`: manifest generation (scoped
  `GeneratePreservePrompt`), `parent_session_id` wiring, `~/.agm/handoffs/`
  persistence, `vroom.handoff.created` topic, and the `captured_at_ref`
  staleness banner.

---

## References

- [ADR-002: VROOM Execution Architecture](ADR-002-vroom-execution-architecture.md)
- [ADR-007 (agm): Hook-Based State Detection](../../agm/docs/adr/ADR-007-hook-based-state-detection.md)
- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md)
- [ADR-012: Provider Transport Layer](ADR-012-provider-transport-layer.md)
- [ADR-018: Graceful Exit as a Framework Default](ADR-018-graceful-exit-framework-default.md)
- [ADR-022: Backlog Suggestion System](ADR-022-backlog-suggestion-system.md)
- [pkg/engram ADR-003: Memory Strength Tracking Fields](../../pkg/engram/docs/adrs/ADR-003-memory-strength-tracking-fields.md)
- [/CONTEXT.md](../../CONTEXT.md) — VROOM vocabulary, DEAR loop, terminology collisions
- `agm/cmd/agm/send_compact.go`, `agm/cmd/agm/session_compact.go` — the
  existing compaction surface this ADR is careful not to duplicate.
- `agm/cmd/agm/admin_link_session_parent.go` — the `parent_session_id` chain
  reused for handoff custody (§H4).
