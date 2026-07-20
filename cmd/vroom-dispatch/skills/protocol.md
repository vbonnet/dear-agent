# VROOM Supervisor Protocol

This is the shared contract for the Meta-Orchestrator, Orchestrator, and
Overseer. Role files define only the work unique to that role.

## Sources of truth

- **Work:** Beads at `~/beads/context-engine/.beads`, read with `bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <subcommand>`.
- **Execution:** live AGM sessions and their health records.
- **Delivery:** GitHub PR state, deployed-artifact state, and verification
  evidence recorded on the bead.
- **Topology:** `pkg/vroom/supervisor`, exposed through `agm supervisor`.

Do not create or consume `roadmap.jsonl`, `dispatched.jsonl`,
`deploy-dispatched.jsonl`, or prompt files as operational state. They are
retired projections that can disagree with Beads, sessions, and PRs.

## Canonical commands

Use these commands as written. Do not hide stderr; a failed observation must be
visible to the tick and must fail closed.

```bash
agm supervisor heartbeat --id <supervisor-id>
agm supervisor status
agm -o json session health --all
agm -o json scan --cross-check
agm supervisor probe
bd --db ~/beads/context-engine/.beads --dolt-auto-commit on ready --json
```

The Orchestrator dispatches only through the typed direct dispatcher:

```bash
~/go/bin/vroom-dispatch-direct \
  -db ~/beads/context-engine/.beads \
  -repo vbonnet/dear-agent
```

The dispatcher owns Beads parsing, live-session and open-PR deduplication,
prompt rendering, JSON decoding, and spawn backpressure. A role prompt must not
reimplement those decisions with shell pipelines.

## Permission safety

`agm scan --cross-check` is the only supervisor permission-recovery path. Its
typed classifier may auto-approve a prompt only when the captured action matches
the configured RBAC allowlist.

Supervisors and workers must never issue a manual permission approval. When a
prompt remains stuck after the cross-check, report the session and captured
action to a recovery peer or raise an asynchronous `blocked-action` escalation.
Defer, reject, or escalate unknown actions; never convert uncertainty into
approval.

Detached sessions use their configured automatic mode and role profile to avoid
routine prompts. A prompt outside that profile is a policy defect or an
unexpected action, not routine backpressure.

## Completion contract

A delivery bead is complete only when all applicable gates have evidence:

1. **Merged:** its PR is `MERGED` with a non-null `mergedAt`.
2. **Deployed:** any changed deployable artifact is installed from the merged
   revision and its status check is clean. Record `N/A` with a reason when the
   change has no deployable artifact.
3. **Verified:** relevant source checks pass and, for runtime changes, the
   installed behavior is exercised locally.

“PR created,” “approved,” and “merge queued” are intermediate states. Never
close a bead on those states. Product, security, money, legal, destructive, or
other human-owned merge/deploy decisions remain blocked until the human acts.

Terminal bead notes begin with exactly one status:

- `DONE:` all applicable gates above passed, with evidence.
- `DONE_WITH_CONCERNS:` all gates passed, followed by a concrete reservation.
- `FAILED:` delivery is incomplete, followed by the blocker and alternatives.

## Tick contract

At the start of every tick:

1. Write the role heartbeat.
2. Read peer status from `agm supervisor status`.
3. Drain pending escalations with `agm escalate list --mine --pending`.

Every subprocess result is data. On a command or decoding failure, skip the
dependent action, report the error, and finish the turn normally so the recurring
loop continues. Never dispatch, approve, close, deploy, or reclaim from missing
or ambiguous state.

## Shared constraints

- Never write to `~/src/**`; use a worktree for repository changes.
- Always pass the canonical database path to the Beads CLI.
- Use `safe-push` and `safe-merge`; never pass force or verification-bypass
  flags.
- Never build JSON or JSONL with shell interpolation. Use typed Go commands and
  their structured output.
- Do not use Python for repository automation.
- Treat a circuit-breaker refusal as expected backpressure: report it and retry
  on a later tick.
- Keep acting within the role's authority without waiting for routine
  confirmation. Escalate only at an explicit authority or safety boundary.
