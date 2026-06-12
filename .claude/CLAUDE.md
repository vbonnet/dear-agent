# dear-agent — Project Instructions

## Core Engineering Principles (MANDATORY)

These nine principles govern *how* work happens in this repo, regardless of
*what* the task is. They were distilled from the 2026-05-28 working session
and version-controlled here so every agent on every machine inherits them.
The more specific sections below (Output Routing, Dogfooding, Agent
Delegation Enforcement) are instances of these principles, not exceptions to
them.

1. **No scope creep — one agent, one scoped plan.** "Also fix this while
   you're here" is banned. Each concern gets its own dedicated agent with its
   own dedicated plan. If you notice an unrelated problem mid-task, file it
   (Beads issue, spawned task, or retro action item) and stay on your plan.
   *Why:* bundled changes are unreviewable, un-revertable, and dilute the
   audit trail. Scoped work is the unit the rest of these rules operate on.

2. **Enforcement is positive guidance, not punishment.** When you write a
   hook, deny-rule, or error message, phrase it as *"You're trying to do X.
   Here's the right way: [instructions], because [reason]."* — never a bare
   *"You can't do X."* Always explain the *why* so the blocked agent can
   generalize to cases the rule didn't anticipate. *Why:* a rule with no
   rationale gets routed around; a rule that teaches gets internalized.

3. **Broken thing? DEAR retro → new agent → scoped plan. Never fix in-line.**
   When you hit a process gap or systemic defect, do not patch it in the
   middle of the current task. Write a Define→Execute→Audit→Retro entry in
   `docs/retros/`, let the retro produce action items, then spin up a
   dedicated worker to execute a scoped plan from those findings. *Why:*
   in-line fixes are invisible, untested, and recreate the scope-creep that
   principle 1 forbids.

4. **No Python for anything we control. Go is the default.** Write Go.
   Rust or TypeScript are permitted *only* with strong, stated justification
   (e.g. an ecosystem that has no Go equivalent). Python is not an option for
   code we own and ship. *Why:* a single primary language keeps the codebase
   navigable, the toolchain (`make preflight`, `lint`, `vulncheck`) uniform, and
   the build reproducible.

5. **Use `/wayfinder` for all plans and execution.** Plan and drive
   consequential work through the Wayfinder SDLC workflow rather than ad-hoc.
   *Why:* dogfooding Wayfinder feeds the self-improvement flywheel — every
   real run surfaces gaps before users hit them. (This is the planning-side
   companion to the Dogfooding rule below.)

6. **Always route through dear-agent / AGM / VROOM.** Every task is also a
   data point for our own infrastructure. Spawn work via AGM, route
   governance-relevant decisions through the VROOM supervisory mesh, and
   prefer our surfaces over bypassing them. *Why:* routing around our own
   tooling silently widens the gap between what we ship and what we trust.
   See [Dogfooding — Use AGM and VROOM](#dogfooding--use-agm-and-vroom-mandatory) below for the operational detail.

7. **JIT access model — escalate permission blocks into fixes.** When you hit
   a permission/access block, do not work around it and do not retry. Stop,
   escalate, run a mini DEAR retro on *why the permission model lacked this*,
   and fix the model so **every** agent gains the access — not just a one-off
   grant for you. *Why:* a per-incident workaround leaves the next agent
   blocked on the same wall; fixing the model retires the class of failure.

8. **Track everything in Beads — single source of truth.** Add new work to
   the tracker the moment you discover it; update task status on completion;
   never let real work live only in your head or a scratch file. The
   canonical store is the `context-engine` Beads DB (`~/beads/context-engine`).
   *Why:* untracked work is invisible work — it cannot be prioritized,
   handed off, or audited.

9. **Atomic action wrappers — wrap unsafe command chains, deny the raw form.**
   When an action only succeeds as an all-or-nothing chain (e.g. `chezmoi
   apply` → commit → push), or when a raw command cannot be permission-granted
   without over-granting (e.g. allowing `git push` also allows `git push
   --force`), do not trust an agent to sequence the steps correctly or to
   avoid the dangerous flags. Build a wrapper instead:
   - Write a small, deterministic wrapper — a shell script (< 50 lines) or Go
     binary (< 200 lines) — that performs the whole action as one unit.
   - Enforce safety by construction: strip dangerous flags, chain the steps in
     the required order, and roll back (or fail loudly into a clean state) if
     any step fails. The "do A then B then C, and B must not be skipped"
     guarantee lives in code, not in agent discipline.
   - Deny the raw command via a `PreToolUse` hook that exits with code 2 and
     points the agent at the wrapper.
   - `ALWAYS_ALLOW` the wrapper — its safety is guaranteed by construction, so
     it needs no per-invocation approval.
   Examples: `chezmoi-deploy` (`chezmoi apply` → commit → push atomically);
   `safe-push` (`git push` with force-push stripped); any future "do A then B
   then C, and B must not be skipped" action. *Why:* an agent merely *told* to
   run three commands in order will eventually run two of them, or reach for a
   forbidden flag under pressure. A wrapper makes the safe path the only path,
   and turns a fuzzy permission question ("can this agent push?") into a crisp,
   auditable one ("can this agent run the binary we vetted?"). Prefer building
   the wrapper over loosening a permission rule.

## Output Routing — Where Artifacts Belong (MANDATORY)

This repo holds **code**, not research. Research artifacts (analysis docs,
transcripts, literature reviews, findings) belong in `engram-research`.
Conversation logs belong in `ai-conversation-logs`. Routing is governed by
`.dear-agent.yml` at the repo root — read it once at the start of any
session that produces artifacts.

**Forbidden in dear-agent** (declared by `.dear-agent.yml > forbidden-paths`):
- New `*.md` or `*.txt` files under `research/`. dear-agent does not
  currently have a `research/` tree, and any such file should be redirected
  to `~/src/engram-research`.

**Where things go:**

| Artifact kind                                              | Destination                  |
|------------------------------------------------------------|------------------------------|
| Source code, ADRs (`docs/adr/`), design docs (`docs/design/`) | this repo                |
| Research analysis (substrate/architecture studies, etc.)   | `~/src/engram-research`      |
| Source transcripts (YouTube, podcasts, interviews)         | `~/src/engram-research`      |
| Conversation/session logs                                  | `~/src/ai-conversation-logs` |

**Decision procedure** when writing a new file:
1. If it is code, build config, ADR, or design doc that constrains code in
   this repo → write here.
2. Otherwise check `.dear-agent.yml > output-dirs` for the matching kind and
   write there instead.
3. If unsure, ask the user — do **not** default to `research/` in this repo.

This rule exists because research artifacts were committed to the predecessor
code repo (ai-tools) in error multiple times, polluting code-repo history and
stranding work away from the corpus where it belongs. Treat the redirect as
authoritative.

See [AGENTS.why.md](../AGENTS.why.md) for the rationale behind the two-tier
(instruction + configuration) routing model.

## Dogfooding — Use AGM and VROOM (MANDATORY)

This repo *is* AGM and VROOM. Every task here is also a chance to exercise
the very tooling we ship. Default to running work through our own surfaces
instead of bypassing them.

**When to dogfood — by default, for any non-trivial task in this repo:**

- **AGM** for session orchestration: spawn isolated work via
  `agm session new` / `agm send` instead of opening ad-hoc terminals;
  read the `acceptance-criteria:` block of `.dear-agent.yml` at the
  start of a task (the `pkg/acceptance` loader formalizes it — there is
  no `agm acceptance` subcommand) and check `agm admin doctor` if
  something looks off.
- **VROOM** for multi-step or governance-relevant work: route consequential
  decisions through the supervisory mesh (the MISSION.md framework), so the
  append-only audit log captures rationale and gates.
- **Define → Execute → Audit → Retro (DEAR)** loop: when finishing a
  non-trivial change, write or update the matching artifacts in
  `docs/retros/` if the change exposes a process gap.

**Why this is a rule, not a suggestion:** dogfooding surfaces real gaps
before users hit them. Every time we route around our own tools, we lose a
data point and silently widen the gap between "what we ship" and "what we
trust." If a tool is too painful to use on its own repo, that pain is a bug
to file (or fix), not a reason to bypass.

**Acceptable bypass:** trivial single-file edits, one-shot reads, and the
literal bootstrap case where the tool itself is broken (in which case: file
an issue or write a retro before moving on).

## Anti-Stall — Continuous Execution (MANDATORY)

**Keep going. The default is to continue.** When you are working a
backlog, a plan, or a multi-step task, do the next item — do **not** stop
to ask "should I keep going?". The human is watching and will interrupt
if priorities changed; asking permission to continue work you were
already asked to do is the stall this rule exists to prevent.

The full behavioural contract — with the boundary cases where stopping
*is* correct — is the single authoritative spec at
[docs/design/anti-stall.md](../docs/design/anti-stall.md). Read it once
per session that does multi-step work. Its five directives:

1. **Continue through backlogs without asking.** More items in the
   plan/backlog → do the next one. Never ask whether to pick up a backlog
   item; just do it.
2. **"Nothing found" is always a valid outcome.** Never inflate a weak
   match to avoid an empty result, and never stall asking whether empty
   is acceptable — it is (see [graceful-exit.md](../docs/design/graceful-exit.md)).
3. **Present decisions, not questions.** At a fork, decide and state the
   decision with a clean interrupt point ("using A because B is blocked;
   say so if you'd rather B") instead of asking which way to go.
4. **Minimize blocking on human input.** Resolve from context, code, and
   defaults first; batch genuinely necessary questions; keep working on
   the unblocked parts.
5. **If genuinely blocked, file it and move on.** Create a Beads task for
   the blocker, note it in your summary, and pick up the next independent
   item — do not idle the whole backlog on one stuck item.

This is the **keep-going** half of the contract; the section below is the
**when-to-stop** half. They are complements, not contradictions: stop for
supervisor commands, repeated failure, irreversible actions, and
decisions only a human can make — never for permission to continue.

## Agent Delegation Enforcement (MANDATORY)

These rules come from the 2026-05-13 DEAR retro on stuck tasks
(`~/ai-conversation-logs/dear-retros/2026-05-13-enforcement-rules.md`).
The pattern they correct: long agent runs that produced uncommitted work,
ignored supervisor pings, retried the same failing approach indefinitely,
and left worktrees and feature branches stranded after merge. Turn budgets
were considered and rejected — they are training wheels that punish careful
work and reward rushed work. The discipline below is causal: commit early,
listen to the supervisor, stop retrying, clean up.

### 1. Incremental commit discipline

**Uncommitted work is nonexistent work.** Commit after each logical
sub-task — not at the end, not when "everything is perfect." If the worker
process is killed (OOM, timeout, supervisor stop), only what is in git
survives. The cost of an extra commit is ~zero; the cost of losing 90
minutes of work is large.

- First commit within the first meaningful unit of progress (scaffold,
  failing test, skeleton). Do not let the first commit be "everything done."
- Commit on every sub-task boundary. Use clear, conventional messages.
- WIP commits are fine — they can be squashed at PR time.

### 2. Supervisor messages are commands

When an orchestrator/supervisor sends a message (AGM `send`, VROOM
intervention, user redirect), it is a **command**, not a suggestion.
Goal-pursuit does not override it.

- **Acknowledge within 2 turns** of receipt.
- **Comply within 5 turns** — even if compliance means committing WIP and
  returning early.
- `wrap up` → commit current state, return summary.
- `status?` → report progress, remaining work, blockers in one turn.
- `stop` → commit immediately and return. Do not continue.

### 3. Two-retry maximum, then escalate

If an approach fails twice with the same error, **stop**. Do not keep
trying. Retry loops burn time and budget without converging.

- After 2 failures: try a materially different approach, OR report failure
  with two concrete alternatives and ask for direction.
- Permission/access errors: 0 retries. Report immediately — retrying will
  not change the answer.
- Death loops (same error 3+ times in a row) are an immediate stop-and-ask.

### 4. `git push` with timeout and no prompts

On this host, `git push` over HTTPS can hang on a keychain prompt and look
like a network failure (see `memory/macos-env-gaps.md`). Always:

```
GIT_TERMINAL_PROMPT=0 gtimeout 30 git push -u origin <branch>
```

If push fails or times out: **leave the branch local**, report the failure,
and let the supervisor decide. Do not retry with different flags hoping to
get past the prompt.

### 5. Worktree and branch cleanup after merge

A merged branch with a stranded worktree is a leak that compounds over time.
After a successful merge to `main`:

```
git -C ~/src/dear-agent worktree remove <worktree-path>
git -C ~/src/dear-agent branch -D <branch>   # local
git -C ~/src/dear-agent push origin --delete <branch>   # remote, if pushed
```

If `gh pr merge --squash --delete-branch` was used, the remote branch is
already gone — still remove the local worktree and branch.

### 6. Definition of Done includes "committed to branch"

Every delegated task's DoD must **explicitly** list:

- [ ] Changes committed to the working branch
- [ ] (If applicable) Branch pushed to origin
- [ ] (If applicable) Tests + lint pass on the committed tree

A task that says "the code works on disk" but is not in git is **not done**.
Delegation prompts that omit this line have produced the exact failure mode
this section exists to prevent — include it verbatim.

## Living Documentation Policy (MANDATORY)

Documentation in this repo must be **living** — it describes the current
state of the code, not a historical snapshot or a future plan.

### What lives here vs. what goes to engram-research

**In this repo (living docs only):**
- `ARCHITECTURE.md` — structural map of the codebase
- `docs/adr/` — Architecture Decision Records (binding decisions, not plans)
- Inline code comments that explain *why*, not *what*
- API documentation generated from or co-located with source (`SPEC.md`,
  tool-level `--help` strings)
- `CLAUDE.md` — agent instructions that constrain code in this repo

**In `~/src/engram-research` (temporal artifacts):**
- Design docs, problem statements, wayfinder artifacts, DEAR retros
- Research, analysis, literature reviews
- Any document whose value is primarily historical or exploratory

See `.dear-agent.yml > forbidden-paths` for the machine-readable enforcement
(PR #341 is adding `docs/retros/**`, `docs/design/**`, and `wf/**`).

### Docs must live next to the code they document

A document that describes a package should live *in* or *adjacent to* that
package, not in a central docs tree. If you are documenting `pkg/vroom`,
the right place is `pkg/vroom/SPEC.md` or inline doc comments — not a
top-level file that an agent or reader must hunt for.

**CI enforcement intent (tracked as Beads task):** when code in a directory
changes, any co-located documentation should also be reviewed and its
timestamp updated if the content remains accurate. The exact gate is tracked
as a Beads task — see the "doc proximity" rule below.

### The "Last audited at" timestamp contract

Living docs SHOULD carry a header line:

```
<!-- Last audited at: 2026-06-12 -->
```

Updating this timestamp is a **commitment**, not a housekeeping step. It
means: *"I read every claim in this file and verified it matches the current
codebase."* Fixing a typo or adding a sentence does NOT justify bumping the
timestamp unless you also verified the whole file.

If you update the timestamp without a full read, you are forging an audit.
This is worse than a stale timestamp — a stale timestamp is honest; a false
one is noise that hides real drift.

### Stale facts trigger a DEAR retro

If you find a claim in a living doc that is factually wrong or out-of-date:
1. Fix the claim (or delete the doc if it is no longer relevant).
2. Write a one-paragraph DEAR retro entry in `docs/retros/` explaining *why*
   the drift happened and what process gap allowed it.
3. File a Beads task if the retro surfaces an action item.

Do **not** silently fix a stale doc without the retro. Silent fixes hide
process gaps that will produce more drift.
