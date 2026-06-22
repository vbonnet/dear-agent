# VROOM Supervisor Shared Protocol

This document defines the shared state, file formats, and communication
conventions used by all three VROOM supervisors (Meta-Orchestrator,
Orchestrator, Overseer). Each supervisor reads this at boot.

## State Directory

All shared state lives under `~/.agm/vroom/`. Create it if missing:
```bash
mkdir -p ~/.agm/vroom/heartbeat
```

| File | Format | Writer | Readers |
|------|--------|--------|---------|
| `trail.jsonl` | JSONL append-only | All 3 | All 3 + humans |
| `roadmap.jsonl` | JSONL append-only | Meta-O only | advisory log (see note) |
| `heartbeat/meta-o.json` | JSON atomic-write | Meta-O | Orch, Overseer |
| `heartbeat/orch.json` | JSON atomic-write | Orch | Meta-O, Overseer |
| `heartbeat/overseer.json` | JSON atomic-write | Overseer | Meta-O, Orch |

> **Beads are the dispatch source of truth (ce-1jm2).** The Orchestrator
> dispatches **directly from `bd ready`** via `vroom-dispatch-direct` — it no
> longer reads a `roadmap.jsonl` "accepted" projection, and there is no
> `dispatched.jsonl` ledger (both were the retired prompt-file layer). In-flight
> state is derived from live `worker-<id>` sessions + open PRs. `roadmap.jsonl`
> remains an advisory record of Meta-O's prioritization rationale, but it does
> **not** gate dispatch: a Meta-O "reject" only takes effect when Meta-O acts on
> the bead itself (closes it, or sets it blocked/low-priority), which is what
> removes it from `bd ready`.

## Record Formats

### Trail Record (all supervisors write)
One JSON object per line, appended via `>>`:
```json
{"ts":"2026-06-15T22:00:00Z","role":"meta-orchestrator","kind":"supervisor.metao.roadmap.evaluated","payload":{"bead_id":"ce-abc1","title":"Fix X","accepted":true}}
```
Fields: `ts` (RFC3339 UTC), `role` (your role name), `kind` (event topic), `payload` (free-form object).

Write via:
```bash
printf '{"ts":"%s","role":"%s","kind":"%s","payload":%s}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "<role>" "<kind>" '<json>' \
  >> ~/.agm/vroom/trail.jsonl
```

### Roadmap Record (Meta-O writes — advisory only)
```json
{"bead_id":"ce-abc1","title":"Fix auth timeout","priority":"P1","state":"accepted","reason":"Blocking 3 other beads","decided_at":"2026-06-15T22:00:00Z"}
```
States: `accepted`, `rejected`. This is an advisory rationale log; it no longer
gates dispatch (ce-1jm2). To actually keep a bead out of dispatch, Meta-O must
act on the bead in `bd` (close it, or mark it blocked/low-priority) so it drops
out of `bd ready`.

> The `dispatched.jsonl` ledger that previously recorded each dispatch was
> retired with the prompt-file layer (ce-1jm2). Dispatch is now derived from live
> `worker-<id>` sessions and open PRs; the `supervisor.orch.dispatched` trail
> record remains the audit trail.

### DoD Audit Trail Kinds

Definition-of-Done enforcement (a bead is Done only when its PR is MERGED — see
the DEAR retro that found ce-6f1b, ce-mcw2, ce-1onr closed against unmerged work)
emits two dedicated trail kinds:

- `dod.audit.violation` (Overseer) — a recently-closed bead was closed against a
  PR that is not merged. Payload fields: `bead`, `pr`, `note`.
- `dod.dispatch.blocked` (Orchestrator) — a bead's dispatch was held because one
  of its closed dependencies was closed against an unmerged PR. Payload fields:
  `bead`, `dep`, `pr`, `dep_pr_state`.

Both are written with the standard trail append. A human (or Meta-O) scanning the
trail for `dod.` prefixes sees every closure-discipline breach in one grep.

### Dispatch Concern Trail Kind

When a worker finishes its deliverable but holds a reservation (see
[Worker Status Codes](#worker-status-codes)), the Orchestrator records it so a
human or supervisor can flag the work for review:

- `supervisor.dispatch.concerns` (Orchestrator) — a worker signalled
  `DONE_WITH_CONCERNS` in its bead notes. Payload fields: `bead`, `session`,
  `concern` (the worker's verbatim reservation, or a short summary), and
  optionally `pr` and `verifier` (the session spawned to double-check, if any).

A human grepping the trail for `supervisor.dispatch.concerns` sees every
"complete, but…" deliverable in one pass — these are the beads most worth a
second look even though they were not failures.

## Worker Status Codes

Every worker reports a terminal outcome in its bead notes when it stops working a
bead. There are exactly three:

| Code | Meaning |
|------|---------|
| `DONE` | Deliverable complete, no reservations. The bead's PR is merged (DoD satisfied) and the worker has nothing it wants flagged. |
| `DONE_WITH_CONCERNS` | Deliverable complete, **but** the worker holds a reservation about some aspect (a risky assumption, a shortcut taken under time pressure, a test it could not run, a design tradeoff it is unsure about). The worker MUST document the concern explicitly in the bead notes — what the reservation is and why — so a supervisor or human can decide whether to act on it. The deliverable still ships; the concern is a flag, not a blocker. |
| `FAILED` | Deliverable not complete. The worker could not resolve the bead and reports the blocker plus concrete alternatives. |

`DONE_WITH_CONCERNS` exists so a worker is never forced to choose between
silently shipping work it has doubts about (the doubt is lost) and failing a
bead it actually completed (the work is discarded). It surfaces the doubt
without throwing away the deliverable.

**Worker convention — recording the outcome.** Add a bead note whose first token
is the status code, then close (or leave open per DoD) accordingly:
```bash
# Complete but with a reservation — document the concern, then proceed with DoD closure.
bd --db ~/beads/context-engine/.beads note <bead-id> \
  "DONE_WITH_CONCERNS: <one-line reservation>. Detail: <why this is a concern / what was not verified / what assumption was made>."
```
The status code MUST be the first token of the note so the Orchestrator can
detect it with a simple grep when the worker session ghost-exits.

## Heartbeat

Each supervisor writes its heartbeat using the existing AGM command:
```bash
agm supervisor heartbeat --id <supervisor-id> --primary-for <peer> --tertiary-for <peer>
```

This writes `~/.agm/supervisors/<id>/heartbeat.json`. Additionally, write a
simpler file for quick peer checks:
```bash
date -u +%Y-%m-%dT%H:%M:%SZ > ~/.agm/vroom/heartbeat/<name>.json
```

## Peer Liveness Check

Read the peer's heartbeat file. If missing or timestamp is >5 minutes old,
the peer is **stale**. Actions:
1. Record to trail: `kind: "supervisor.<role>.peer_stale"`
2. Send message: `agm send msg <peer> --priority urgent --sender <self> --prompt "status? Heartbeat stale."`
3. If >10 minutes: send critical priority

## Inter-Supervisor Messaging

For direct messages between supervisors:
```bash
agm send msg <target-session> --sender <self> --priority <level> --prompt "<message>"
```

Priority levels: `critical`, `urgent`, `normal`, `background`, `fyi`.

## Handling Escalations (Escalate-To-Supervisor)

Workers escalate questions/decisions they cannot resolve up the spawn chain
(ADR-032). As a supervisor you DRAIN escalations on your own tick — never block
your loop waiting on one. Each tick:

```bash
agm escalate list --mine --pending          # escalations currently held by you
agm escalate show <id>                       # full context for one
```

For each, decide:
- **You can answer confidently** (you have the information and it is within your
  authority): `agm escalate answer <id> "<answer>"`. The answer is delivered
  back to the worker automatically.
- **You cannot, or it is above your authority**: `agm escalate forward <id>
  --note "<why / your recommendation>"`. It moves one hop up the chain
  (eventually to the VROOM trio, then the human via Dispatch).
- **Flagged must-reach-human** (product/pricing/destructive/legal/…): you may
  NOT answer it — `forward` with your recommendation so the human decides.

Auto-answerable confirmations ("should I proceed with the task you assigned
me?") never reach you — the tool resolves them at the source.

## Beads Access

Always use the canonical form with explicit database path:
```bash
bd --db ~/beads/context-engine/.beads <subcommand>
```
Never use bare `bd` — it silently resolves to the wrong database.

## Constraints (ALL supervisors)

- **NEVER** write to `~/src/**` (read-only golden checkouts)
- **NEVER** use `--no-verify` or `--force` flags
- **NEVER** use bare `bd` without `--db`
- **ALWAYS** use `GIT_TERMINAL_PROMPT=0 gtimeout 30` for git push operations
- **ALWAYS** write one JSON line at a time to JSONL files (atomic append)
- **ALWAYS** write heartbeat at the START of each tick, right after the peer check (proves the loop is alive and the LLM is responsive; end-of-tick writes cause false STALE when ticks run longer than the 5-minute staleness threshold)

## Tick Resilience (ALL supervisors)

Your tick runs inside a persistent `/loop`. A single failed tick must **never** kill
the loop — if it does, you go silently idle until a human notices.

- **NEVER** let an error end, exit, or abort the loop. If any tick step fails for any
  reason (an Anthropic API or credit-gate error, a tool failure, or a transient fault),
  treat it as a **skipped** tick, not a fatal one, and finish the turn normally so the
  next interval still fires.
- **Best-effort** record the failure to `~/.agm/vroom/trail.jsonl` with
  `kind` = `"supervisor.tick.error"` and a short `payload` describing what failed — but
  if the log write itself fails, ignore it and keep going; logging must never end the loop.
- Recovery is automatic: the next scheduled tick retries the work. Do not attempt to
  tear down or re-create the loop yourself.
