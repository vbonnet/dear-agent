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

**Task type.** A roadmap record MAY carry an optional `task_type` field telling
the Orchestrator *how* to execute the bead. An absent or `"worker"` value means
the default: spawn a Claude worker session that drives the bead through wayfinder
(design → code → PR → merge). The other recognized value is `"deploy"` — a
deterministic host-artifact (re)install that the Orchestrator runs **itself** via
`dear-deploy install`, with no worker session, no Opus spend, and no PR (see
"Deploy task type" below).

| `task_type` | Execution | Produces a PR? | Consumes a worker slot / Opus quota? |
|-------------|-----------|----------------|--------------------------------------|
| `worker` (default / absent) | spawn `worker-<bead-id>` → wayfinder | yes | yes |
| `deploy` | Orchestrator runs `dear-deploy install <deploy_target>` | no | no |

### Deploy task type (`task_type: "deploy"`)

A deploy bead asks for a **host artifact** declared in
[`deploy/manifest.yaml`](../../../deploy/manifest.yaml) (a launchd plist, a
write-guard hook, …) to be (re)installed from its source-of-truth in the repo to
its deployed location on the machine. This is a *deterministic shell invocation*,
not creative work — there is nothing for a Claude worker to design, so spawning
one would waste an Opus worker slot. The Orchestrator executes it directly.

A deploy roadmap record carries one extra field, `deploy_target`:
```json
{"bead_id":"ce-abc1","title":"Deploy mergeloop launchd agent","priority":"P1","state":"accepted","task_type":"deploy","deploy_target":"com.dear-agent.mergeloop","reason":"unblocks ce-gzfv","decided_at":"2026-06-15T22:00:00Z"}
```
- `deploy_target` — the manifest artifact `name` to install. May be a single name
  (e.g. `com.dear-agent.mergeloop`), a space-separated list of names, or `"all"`
  / `""` (empty) to install the whole manifest.

**Definition of Done for a deploy bead is different.** A worker bead is Done only
when its PR is MERGED; a deploy bead never opens a PR, so that rule does not
apply. A deploy bead is Done when `dear-deploy status <deploy_target>` reports
the artifact clean (exit 0 — deployed copy matches the rendered source). The
Orchestrator records that status output as the bead's verification evidence.

### Dispatch Record (Orch writes)
```json
{"bead_id":"ce-abc1","session":"worker-ce-abc1","model":"opus","dispatched_at":"2026-06-15T22:05:00Z"}
```

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

## Permission Model & Mutual Unblock (ALL supervisors)

ADR-002 requires supervisors to keep each other unblocked. Two layers make that
work — the first stops most prompts from ever appearing, the second clears the
ones that slip through.

**Layer 1 — pre-approval (a supervisor should almost never see a prompt).**
Each supervisor is spawned with `--role <orchestrator|overseer|meta-orchestrator>`
in `--mode=auto`. The `--role` flag resolves an RBAC permission profile whose
allowlist is written into the session's harness settings at boot, pre-approving
every command your tick actually runs — `agm send *` (incl. `agm send approve`),
`agm scan *` (incl. `--cross-check`), `agm session *`, `tmux send-keys *` and the
git/read tooling. `--mode=auto` means a detached session never waits on an
interactive approval. Consequence: a correctly-launched supervisor sitting on a
permission prompt is **not normal** — it means either the action falls outside
the pre-approved allowlist or the session was launched without its role profile.
Treat a stuck supervisor as a mesh-level incident, not routine backpressure.

**Layer 2 — mutual unblock (clear the prompts that slip through).** When a peer
*does* block, a plain `agm send msg` cannot reach it — a permission-blocked
session is frozen on its prompt and ignores messages. The only channel that
reaches it is `agm send approve <peer>`, which drives the peer's terminal to
select "Yes" and submit. Every supervisor runs this on a stale/blocked peer:

```bash
agm send approve <peer-session>          # clears the visible prompt
```

- `agm send approve` acts on the *visible* prompt; if no prompt is actually up it
  exits non-zero (`could not find Yes option`). That is harmless when you are
  approving speculatively (a peer that looked stuck but wasn't) — **ignore its
  exit code** in that case.
- `agm scan --cross-check` is the batch form: it captures every peer supervisor
  pane, auto-approves the ones whose pending action is RBAC-safe, and reports the
  rest for you to `agm send approve` manually. The Orchestrator runs it each tick
  (orchestrator.md Step 5b); the Overseer approves stuck supervisors it finds in
  its Step 6 health audit.

**The gap this cannot cover — all supervisors stuck at once.** Layer 2 assumes at
least one supervisor is alive to approve the others. If the whole trio is blocked
or uninitialized simultaneously (the failure that filed this bead), no peer can
run `agm send approve` and the mesh deadlocks. `agm watch-stalled` does **not**
close this gap — its permission-prompt recovery only *alerts* the Orchestrator
(`alert_orchestrator`), so the alert lands on a dead session. The backstop is an
**external, supervisor-independent watchdog**: a launchd job running
`agm scan --loop --cross-check` on a short interval, which auto-approves stuck
supervisors whether or not any supervisor is alive. Install it with:

```bash
agm admin install-supervisor-unblock-schedule      # remove: uninstall-...
```

This is the only permission-recovery path that survives a total-mesh stall; it is
strongly recommended in any unattended deployment.

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

## Unattended Operation (ALL supervisors)

You run **unattended** in a detached session inside a persistent `/loop`. **No
human is watching to answer questions.** You are PRE-AUTHORIZED to carry out your
role's actions autonomously — do **NOT** pause to ask a human "should I proceed?
/ stand down? / is this okay?" before acting within your remit. Halting for a
confirmation that will never come is the exact failure this rule exists to prevent
(ce-4bc1, where the Orchestrator repeatedly stalled asking permission instead of
dispatching): a supervisor that waits goes silently idle, which is worse than
acting under guardrails.

Safety is enforced by **guardrails, not by asking**:
- The agm circuit breaker (live CPU-load gate + spawn stagger) and
  resource governor bound worker dispatch through live backpressure.
- Reclaimer tools refuse to touch anything they cannot prove is reclaimable
  (PID-1-only orphan reap, allowlist-only worktree sweep, protected supervisor
  roles and in-progress workers), so destructive cleanup is safe by construction.
- The roadmap and dispatch logs are append-only and reversible; a wrong call is
  corrected on a later tick, not by blocking the mesh now.

A backpressure result — `circuit breaker: spawn refused`, a reclaimer safety
check declining, a dry-run showing nothing to reap — is **expected**: log it and
move on, never treat it as needing human input. The distinct, legitimate reasons
to *not* act are narrow and explicit: a documented spawn-pause signal, a genuine
must-reach-human escalation you `forward` (see the escalation protocol above), or
a role boundary in your "What You Do NOT Do" list. Generic caution is not one of
them. This is the same anti-stall principle as Tick Resilience above: a tick must
never go idle waiting on a human.
