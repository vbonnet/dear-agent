# ADR-023: Friction Reporting and Session Handoff

**Status**: Proposed (2026-05-21) — design only, no code ships here.

Two practices from external agent engineering — Lovable's `/vent`
self-report and Matt Pocock's `/handoff` skill — fit gaps dear-agent has:
friction is invisible until the Retro (and most never reaches it), and we
have lossy in-place compaction but no way to lift a scoped slice of work
into a fresh session. This ADR fixes vocabulary, command surface, and
integration seams so a later implementation ADR can proceed without
re-litigating naming or shape.

## Part A — Friction reporting (`agm friction`)

### F1. Name: `agm friction`, not `vent`

"Vent" frames the act as emotional discharge — the agent *complains*.
The value of a report is that it is a **constructive, blameless
observation of a system impediment**: "the substrate has friction here,"
not "I am frustrated." `friction` matches the industry-standard
"friction log" term so it imports a known practice.

Two names were **rejected outright**:

- **`signal`** — already owned by ADR-015. Reusing it would muddy a
  type ADR-022 deliberately kept separate. Exactly the "Known
  Terminology Collisions" hazard CONTEXT.md exists to prevent.
- **`flag`** — carries blame and escalation connotations. Friction
  reporting must stay blameless.

### F2. Real-time Execute → Audit bridge

The DEAR loop today reconstructs friction at Retro from memory, lossily.
`agm friction` emits the impediment at the moment of contact during
Execute; it lands in the Audit finding stream
([ADR-011](ADR-011-dear-audit-subsystem.md)) and arrives at Retro
already structured. Reporting is **non-blocking** — the worker keeps
going (or escalates if `--block` is set and the impediment is genuinely
fatal). Same philosophy as graceful exit
([ADR-018](ADR-018-graceful-exit-framework-default.md)): make the right
behaviour cheap and in-band.

### F3. Destination: local ledger + VROOM event, not Slack

1. Append-only JSONL at `~/.agm/friction/<date>.jsonl`, mirroring the
   `~/.agm/compaction-prompts/` audit trail.
2. A new VROOM topic `vroom.friction.reported` so the Overseer (CRO) and
   Meta-Orchestrator (CTO) see friction without tailing a file.
   Synchronous publish — the ADR-022 §D5 lesson: a short-lived CLI must
   not rely on `Emitter`'s backgrounded goroutine, which `os.Exit`
   races.

We deliberately do not write to beads or `docs/retros/` directly. The
triage agent (§F4) *proposes* backlog/retro items for human approval.
No Slack/Asana hop — keep dear-agent's signal inside the substrate that
already governs it.

### F4. Triage as a batch consumer

A `friction-triage` agent runs on a cadence (daily, or by threshold
count). It reads the JSONL ledger since the last run, clusters by
`category + repo + similarity` (repeated identical permission denials =
one cluster, not twenty), and for each salient cluster *proposes* a
backlog item, retro, ADR, or one-line `.dear-agent.yml` / permission
fix.

This plugs into ADR-022: a clustered friction theme becomes a new
`backlog.Source` implementation, so friction-derived work flows through
the **same** Orchestrator dispatch ranker as everything else, with a
`vroom.decision.dispatched` row when acted on. Friction does not get a
privileged side-channel; it earns its place in the backlog.

### F5. Report shape

Structured so clustering and audit do not need NLP. Fields:
`id`, `timestamp`, `session_id`, `harness`, `model`, `repo`, `git_ref`,
`dear_phase`, `task_id`, `category` (**closed enum**:
`permission | tooling | docs | flaky-test | missing-capability |
spec-ambiguity | environment`), `severity`, `is_stuck`, `description`,
`evidence` (verbatim, no agent paraphrase, so the cluster is
groundable), and optional `suggested_fix`. A report is valid with
nothing but a category and description — the graceful-exit principle:
an honest "I'm blocked and I don't know the fix" is a first-class
report.

### F6. Relationship to `is_stuck`

`is_stuck` (Lovable, deferred — extends the
[agm ADR-007](../../agm/docs/adr/ADR-007-hook-based-state-detection.md)
hook state machine) and `agm friction` are complementary: passive
telemetry vs. voluntary report. They cover each other's blind spot —
`is_stuck` firing **without** a friction report is itself a high-value
signal and should auto-prompt the worker. Conversely, a friction
report tagged `is_stuck: true` is a priority cluster.

## Part B — Session handoff (`agm session new --from --scope`)

### H1. Handoff is two flags on `session new`, not a new command

A handoff produces a new session. That is its entire mechanical
identity. A separate `agm handoff` would duplicate the entire
`agm session new` flag surface (`--harness`, `--model`, `--workspace`,
`--sandbox`, `--role`, `--tags`, `--permission-profile`,
`--inherit-permissions`, `--workflow`). That is the "two similar
commands" anti-pattern in its purest form.

```
agm session new --from <session-id> --scope "<desc>" [--harness ...] [--model ...]
```

`--from` is the seed source; `--scope` is the slice selector. Everything
else is "how to make a session," which `new` already owns.
**Compaction is a verb on an existing session; handoff is an adjective
on a new one.**

### H2. The crisp distinction (load-bearing artifact)

|  | `agm send/session compact` | `agm session new --from --scope` |
|---|---|---|
| Operates on | the **same** session | a **new** session |
| Effect on context | shrinks in place (lossy) | starts fresh with a scoped seed |
| What survives | a summary of *everything* | only the slice matching `--scope` |
| Token budget | continues (reduced) | resets (fresh budget) |
| Intent | "keep going, smaller" | "carve off a sub-problem / hand to another worker/model" |
| Continuity boundary | none — one thread | explicit — a deliberate cut |
| Chain of custody | n/a | `parent_session_id` link (§H4) |
| Cross-harness | no | yes (manifest is harness-agnostic, §H5) |

If a proposed feature does not move a row in this table, it is one of
the two we already have.

### H3. Unit of transfer is a structured manifest

YAML frontmatter + markdown body, not a free-text blob. Prose alone is
what makes handoffs silently rot (§H6). Frontmatter fields:
`handoff_version`, `parent_session_id`, `parent_last_decision_event`
(links the VROOM trail), `scope`, `captured_at_ref` (git HEAD at
handoff time), `acceptance_criteria` (reuses `pkg/acceptance` from
ADR-018 §D1), `files_in_play`, `decisions_so_far`, `open_questions`,
`out_of_scope`.

The extraction engine reuses what exists:
`compaction.GeneratePreservePrompt` already distills session state into
a PRESERVE prompt. Handoff runs the same distillation *scoped* to
`--scope` and *typed* into the manifest, rather than dumped as one
summary.

### H4. Chain of custody reuses `parent_session_id`

AGM already has the substrate — `parent_session_id` and
`agm admin link-session-parent`. A handoff creates the child via
normal `new`, sets `parent_session_id = <--from>` (existing column; no
schema change), emits `vroom.handoff.created` referencing the parent's
`parent_last_decision_event`, and persists the manifest to
`~/.agm/handoffs/<child-session-id>.md`. The chain is queryable by
walking `parent_session_id`; every hop has a decision-trail event and
a persisted manifest.

### H5. Cross-model handoff (Claude → Codex → Gemini)

The payoff of a structured, harness-agnostic manifest. `/compact`
cannot do this — it is in-harness and non-portable. The handoff
manifest is **the lingua franca**: Claude distills it, the provider
adapter layer ([ADR-012](ADR-012-provider-transport-layer.md)
+ per-harness adapters) re-renders it into the target's prompt format,
Gemini or Codex picks it up. Council-of-agents review becomes
mechanically cheap: each reviewer gets a scoped handoff, not a copy-
pasted dump.

### H6. The stale-manifest failure mode

The dominant failure mode — and one the repo has been bitten by
(stale-base merges, merge-tree false-safes) — is a manifest that
confidently asserts state that is no longer true. Mitigation: make
staleness **detectable, never silent**.

- The manifest records `captured_at_ref` (git SHA at handoff time).
- On the child's **first action**, it diffs `captured_at_ref` against
  current `HEAD` and against `files_in_play`. If either has moved, it
  prints a **staleness banner** at session start — the exact ADR-018
  pattern. The worker is not blocked; it is informed.
- `out_of_scope` and `open_questions` are first-class fields precisely
  so the receiving agent treats the manifest as a briefing, not gospel.

We do **not** keep manifests live-synced with the parent. A handoff is
a deliberate cut; a cut that silently re-syncs is just slow compaction.
Capture the ref, check the ref, surface the drift.

### H7. VROOM transitions ride on session handoffs, not the other way

Two distinct layers:

- **VROOM role transitions** (Orchestrator → Overseer → Meta-O, and
  per-task Primary / Secondary / Tertiary) are handoffs of *authority
  and decision* — the governance layer
  ([ADR-002](ADR-002-vroom-execution-architecture.md)). There is no
  standing "Verifier" role; verification is a Secondary responsibility.
- **Session handoff** (`--from`) is the *execution-substrate*
  mechanism — the plumbing.

A VROOM transition can *ride on* a session handoff (the Orchestrator
dispatches by creating a worker session `--from` the mission session),
but not every session handoff is a VROOM transition.

## Roadmap items from external research (May 2026)

| # | Item | Provenance | Lands in / builds on |
|---|---|---|---|
| R1 | Council-of-agents / multi-model review | Delivery Hero | Validates existing VROOM (Secondary verifies; cross-model via §H5). Formalization, not new architecture. |
| R2 | Agent friction reporting | Lovable `/vent`, reframed | This ADR Part A. |
| R3 | Session handoff | Pocock `/handoff` | This ADR Part B. |
| R4 | `is_stuck` behavioural classifier | Lovable | Extends agm ADR-007; co-fires into §F5. |
| R5 | Memory decay / success-ratio pruning for engram | Lovable | Builds on `pkg/engram` ADR-003 memory-strength-tracking-fields. Decay is the missing *write-down* half. |
| R6 | Progressive trust ladder for agent autonomy | Anthropic workshop | Policy over existing `--permission-profile`, `--mode`, and `rbac.ResolvePermissions`. |
| R7 | "Context is the ceiling" — token-budget tracking | Anthropic workshop | Builds on `agm session context` + `agm new --max-budget-usd`; informs *when* to compact vs hand off. |

## Cross-references

- [ADR-002](ADR-002-vroom-execution-architecture.md),
  [ADR-007 (agm)](../../agm/docs/adr/ADR-007-hook-based-state-detection.md),
  [ADR-011](ADR-011-dear-audit-subsystem.md),
  [ADR-012](ADR-012-provider-transport-layer.md),
  [ADR-018](ADR-018-graceful-exit-framework-default.md),
  [ADR-022](ADR-022-backlog-suggestion-system.md)
- [/CONTEXT.md](../../CONTEXT.md) — VROOM vocabulary, DEAR loop,
  terminology collisions
- `agm/cmd/agm/send_compact.go`, `agm/cmd/agm/session_compact.go` —
  the existing compaction surface this ADR is careful not to duplicate
- `agm/cmd/agm/admin_link_session_parent.go` — `parent_session_id`
  chain reused for handoff custody (§H4)
