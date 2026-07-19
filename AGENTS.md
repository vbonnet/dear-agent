# dear-agent — Agent Instructions

## Engineering Policies (canonical — read first)

Non-negotiable engineering principles live in [`docs/policies/`](docs/policies/)
as `*.ai.md` (the policy) + `*.why.md` (reasoning + real failure cases) pairs.
They are the source of truth for how every agent — on every harness — works here:

- [broken-windows](docs/policies/broken-windows.ai.md) — delete deprecated code completely, in the same change; never leave two versions coexisting.
- [dear-retro](docs/policies/dear-retro.ai.md) — every systemic defect gets a DEAR retrospective; a fix without a retro recurs.
- [definition-of-done](docs/policies/definition-of-done.ai.md) — done = merged to main, deployed, verified. Not "PR open".
- [wayfinder-v2-canonical](docs/policies/wayfinder-v2-canonical.ai.md) — Wayfinder V2 (9 phases) is the only model; V1 is dead.
- [autonomous-merge](docs/policies/autonomous-merge.ai.md) — review+merge your own PRs, except security/product/money → human.

## Build & Test Commands

```bash
# Build everything
go build ./...

# Build specific binaries
go build -o build/agm ./agm/cmd/agm
go build -o build/engram ./engram/cmd/engram

# Run all tests (matches CI)
go test -race -count=1 ./...

# Run tests for one product
go test ./agm/...
go test ./engram/...
go test ./wayfinder/...

# Run a single test
go test -v ./agm/internal/ops/... -run TestSessionLifecycle

# Run tests without race detector (faster)
go test ./...

# Lint (5m timeout, merge-base regression-only)
golangci-lint run --timeout=5m ./...

# Fast local CI parity (~25s): vet + build + lint
make preflight

# Full CI parity: preflight + tests + race + govulncheck
make preflight-full

# Run only tests affected by your changes vs origin/main
make test-affected

# Validate EARS requirements in SPEC.md files
make lint-specs
```

Test isolation: engram tests that create sessions require
`ENGRAM_TEST_MODE=1` and `ENGRAM_TEST_WORKSPACE=test`. Use
`testutil.RequireTestMode(t)` to enforce this.

CI runs with `GOWORK=off` — there is no go.work file; everything is one root
module.

## Architecture

Four products in one Go monorepo, sharing a single `go.mod`:

| Product | Directory | Purpose |
|---------|-----------|---------|
| **AGM** | `agm/` | Agent Gateway Manager — spawns/monitors/reaps AI agent sessions (tmux-backed) |
| **Engram** | `engram/` | Persistent memory with cue-based retrieval |
| **Wayfinder** | `wayfinder/` | 9-phase SDLC workflow engine |
| **Tools** | `cmd/`, `tools/` | 60+ standalone CLI utilities |

### Three API surfaces, one operations layer

CLI (Cobra), MCP server (JSON-RPC), and agent Skills all route through the
same business logic in `agm/internal/ops/`. An operation implemented once is
available everywhere. `OpContext` provides dependency injection (storage, tmux
client, config, output format).

### Harness adapter pattern

AGM supports multiple AI agent CLIs via adapters in `agm/internal/agent/` —
each implements the `Agent` interface (`Start`, `Stop`, `SendKeys`,
`GetUUID`, `ParseHistory`, `Translate`). Adding a new harness means
implementing this interface; no changes to core ops needed.

### Agent state detection

Session state (READY/THINKING/PERMISSION_PROMPT/COMPACTING/OFFLINE) is
detected via a priority chain: hook execution → tmux pane inspection → manual
tracking.

### Sandbox isolation

Three pluggable providers in `internal/sandbox/`: OverlayFS (Linux), APFS
cloned volumes (macOS), git worktree (fallback). Lifecycle tied to session
lifecycle.

### Framework hierarchy (see CONTEXT.md for full vocabulary)

Wayfinder (planning) → VROOM (supervisory execution) → AGM (session tool) →
DEAR (per-task retro loop). VROOM is *above* AGM — it drives AGM as a tool.
Documents that conflate them are stale.

## Key Directories

- `agm/internal/ops/` — shared operations layer (all three API surfaces)
- `agm/internal/agent/` — harness adapters (Claude, Gemini, Codex, OpenCode)
- `agm/internal/session/` — session lifecycle and manifest management
- `internal/sandbox/` — copy-on-write filesystem isolation providers
- `pkg/` — shared public packages (importable externally)
- `internal/` — private packages
- `codegen/` — code generation framework (separate go.mod)
- `deploy/launchd/` — macOS launch agent plists
- `infra/` — Terraform IaC for GitHub repos/branch protection

## Conventions

- **Go only** — no Python for anything we own. Rust/TypeScript only with stated justification.
- **Conventional Commits** for commit messages.
- **Linting**: `.golangci.yml` uses `new-from-merge-base: origin/main` so only new regressions are reported. Existing violations are baselined and burned down incrementally.
- **Version injection**: binaries use ldflags `-X main.Version/GitCommit/BuildDate/BuiltBy`.
- **Go memory envelope**: `env/go-baseline.env` exports `GOMEMLIMIT=512MiB`, `GOMAXPROCS`, `GOGC` — all build/test/daemon targets inherit it.
- **Atomic wrappers**: unsafe command chains are wrapped in Go binaries that enforce safety by construction (e.g. `safe-push`, `safe-merge`, `safe-rebase`, `safe-pr`). Raw forms are denied via PreToolUse hooks.
- **PR creation**: `safe-pr create --wayfinder <dir>` only — raw `gh pr create` is denied by hook.
- **Temporal artifacts** (designs, retros, wayfinder runs) go to the configured research/knowledge-base destination, never committed to this repo. Living docs (`ARCHITECTURE.md`, ADRs, `AGENTS.md`) stay here.
- **Beads tracker**: always use `bd --db ~/beads/context-engine/.beads <subcommand>` — never bare `bd`.

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
   the configured research/knowledge-base destination, let the retro produce
   action items, then spin up a
   dedicated worker to execute a scoped plan from those findings. *Why:*
   in-line fixes are invisible, untested, and recreate the scope-creep that
   principle 1 forbids.

4. **No Python for anything we control. Go is the default.** Write Go.
   Rust or TypeScript are permitted *only* with strong, stated justification
   (e.g. an ecosystem that has no Go equivalent). Python is not an option for
   code we own and ship. *Why:* a single primary language keeps the codebase
   navigable, the toolchain (`make preflight`, `lint`, `vulncheck`) uniform, and
   the build reproducible.

5. **Use Wayfinder for all plans and execution.** Plan and drive
   consequential work through the Wayfinder SDLC workflow rather than ad-hoc.
   Use the native entrypoint for your harness: Claude slash commands such as
   `/wayfinder:*`, the `wayfinder-session` CLI where available, or AGM/VROOM
   orchestration for non-Claude workers.
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

   **Mandated command form (canonical path is not optional):** always pass the
   canonical database explicitly —

   ```
   bd --db ~/beads/context-engine/.beads <subcommand>
   ```

   Never run a bare `bd` that relies on auto-discovery, and never use `-C`/
   `--db` to point at any other store. *Why — the silent-fallback trap:* `bd`
   resolves its database by walking up from the current directory for a
   `.beads/` dir (like `git`). With no `BEADS_DIR` set, from `$HOME` this used
   to resolve to the tiny `~/.beads` pilot store (prefix `vbonnet-ai`, since
   merged into context-engine and retired), so a bare `bd status` reported
   "backlog drained" while context-engine held ~120 open beads. Reporting an
   empty backlog off the wrong DB is a Definition-of-Done-class failure. The
   `--db` flag makes the right store the *only* store you can hit, and is
   audit-legible in transcripts. A `BEADS_DIR=~/beads/context-engine/.beads`
   default is being deployed as a second layer of defense; the explicit `--db`
   flag remains mandatory regardless.

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

## Documentation & Artifact Routing (MANDATORY)

Every doc this repo touches is exactly one of two kinds. The kind decides
where it lives. Routing is governed by `.dear-agent.yml` at the repo root —
read it once at the start of any session that produces artifacts.

### Temporal artifacts → knowledge base (engram-research), NEVER in dear-agent

A **temporal artifact** captures a *moment of thinking*: it was true when
written and is not maintained as the code evolves. These **always** go to the
knowledge base (currently `~/src/engram-research`, under `projects/<name>/`)
and **never** live in this coding repo — *even when the artifact is about code
in this repo*:

- **Design docs, Plans, Audits, Retros (DEAR), Research, Wayfinder
  artifacts** (`wf/`, W0/problem statements), and the like.

Putting a moment-in-time artifact in code history pollutes the repo, goes
stale silently, and strands the work away from the corpus where it belongs.

### Living documentation → stays in dear-agent

**Living documentation** describes the *current* state of the system and is
maintained in lockstep with the code it documents:

- **`ARCHITECTURE.md`, ADRs (`docs/adr/`), inline/code comments, API docs,
  `AGENTS.md`** — anything kept true as the code changes.

These rules govern living docs:

1. **Proximity principle.** A living doc lives as close as possible to what
   it documents (package-level `doc.go`/`README`/`ARCHITECTURE.md` beside the
   code, not in a far-off `docs/` tree). This is what lets CI enforce
   *"if you touched files in dir X, you must also touch the documentation in
   dir X."* Distance from the code is how docs rot.
2. **`Last audited: <timestamp>` header.** Updating that header is a strong
   claim: *"I read every line of this file and verified it is true right
   now."* It is **not** for typo fixes or partial edits — only bump it after
   a genuine full-content audit of the doc against reality.
3. **A wrong fact in a living doc triggers a DEAR retro.** Treat it as a
   defect, not a typo: do root-cause analysis — hallucination? code changed
   without updating the doc? a fake/sloppy audit? simply too long since the
   last audit? — and let the retro produce action items (per principle 3 of
   the Core Engineering Principles).
4. **Regular doc audits.** Living docs are periodically re-verified against
   reality; a never-audited living doc is assumed stale until proven current.

### Where things go

| Artifact kind                                                       | Destination                  |
|---------------------------------------------------------------------|------------------------------|
| Source code, build config                                           | this repo                    |
| Living docs: `ARCHITECTURE.md`, ADRs (`docs/adr/`), API/inline docs | this repo (next to the code) |
| Design docs, Plans                                                  | `~/src/engram-research`      |
| Audits, DEAR retros                                                 | `~/src/engram-research`      |
| Research / analysis, Wayfinder artifacts (`wf/`), W0s               | `~/src/engram-research`      |
| Source transcripts (YouTube, podcasts, interviews)                  | `~/src/engram-research`      |
| Conversation/session logs                                           | `~/src/ai-conversation-logs` |

**Decision procedure** when writing a new doc:
1. Is it maintained in lockstep with the code and describes the current
   state (architecture, ADR, API, inline)? → it is **living**; write it in
   this repo, as close to the code as possible (proximity principle).
2. Otherwise it is a **temporal artifact** (design, plan, audit, retro,
   research, Wayfinder, W0) → write it to the knowledge base
   (`.dear-agent.yml > output-dirs[research]`, currently
   `~/src/engram-research`), never here.
3. If unsure, ask the user — do **not** default to writing it in this repo.

This rule exists because temporal artifacts were committed to code repos
(the predecessor ai-tools, then dear-agent) in error multiple times,
polluting code-repo history and stranding work away from the corpus where it
belongs. Treat the redirect as authoritative.

> **Pre-existing exception:** a few behavioural *specs* already in
> `docs/design/` (`anti-stall.md`, `graceful-exit.md`,
> `outcomes-framework.md`) are referenced by this AGENTS.md and by code as
> authoritative — i.e. they behave like *living* docs that happen to sit
> under `docs/design/`. They remain in-repo for now; reclassifying them as
> living docs (and giving them `Last audited:` headers) vs. migrating them is
> tracked separately. New design docs follow the table above without
> exception.

See [AGENTS.why.md](AGENTS.why.md) for the rationale behind the two-tier
(instruction + configuration) routing model.

## Dogfooding — Use AGM and VROOM (MANDATORY)

This repo *is* AGM and VROOM. These rules apply across Claude, Codex, Gemini,
Antigravity/agy, OpenCode, and future supported harnesses. Every task here is
also a chance to exercise the very tooling we ship. Default to running work
through our own surfaces instead of bypassing them.

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
  non-trivial change, write or update the matching artifacts in the
  configured research/knowledge-base destination if the change exposes a
  process gap.

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
[docs/design/anti-stall.md](docs/design/anti-stall.md). Read it once
per session that does multi-step work. Its five directives:

1. **Continue through backlogs without asking.** More items in the
   plan/backlog → do the next one. Never ask whether to pick up a backlog
   item; just do it.
2. **"Nothing found" is always a valid outcome.** Never inflate a weak
   match to avoid an empty result, and never stall asking whether empty
   is acceptable — it is (see [graceful-exit.md](docs/design/graceful-exit.md)).
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

## Defer-Don't-Block — Unattended Session Protocol (MANDATORY)

Unattended sessions (overnight burndown workers, scheduled tasks, AGM-spawned
workers) **must never block on ask-gated operations**. If a planned action
requires a permission dialog or interactive approval, deferring it is the
right move — not stalling, not skipping it silently, not retrying until the
session times out.

**The rule:** When a planned operation requires a permission that has not been
pre-approved, defer it and continue:

1. **File a handoff note** — create a Beads task (or a comment on the
   originating bead) describing the blocked operation, why the permission was
   not pre-approved, and what the human needs to do to unblock it.
2. **Continue with the remaining work** — skip the blocked item and keep
   burning down the next independent item on the plan.
3. **Surface the deferred ops in the end-of-run summary** — every unattended
   session's final output must explicitly list any deferred operations so
   the human can review and approve them in the next interactive session.

**Why this is mandatory:** An unattended worker that stalls at 2am on a
permission dialog wastes the entire time budget and leaves the rest of the
backlog untouched. The failure mode is invisible — the worker is "running"
but not doing anything useful. Deferring keeps the flywheel spinning and
makes the blocked item visible.

**What counts as an ask-gated operation:** Any `Bash` command that the
current permission model would surface to the user (e.g. `kill` of foreign
processes, `git commit --no-verify`, raw `gh pr create/close` without the
safe-pr wrapper, write to a chezmoi-managed path). If in doubt, treat it as
ask-gated.

**Pre-approval path (preferred over deferral):** The orchestrator should
check `agm admin doctor` and the bead's required permissions before spawning
a worker. If a task is known to require a permission that isn't pre-approved,
skip it at dispatch time and leave it in the queue rather than letting the
worker discover the block at runtime. This is the longer-term fix tracked in
ce-e3u7 (PermissionChecker revival).

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
safe-push -C ~/src/dear-agent origin --delete <branch>   # remote, if pushed
```

If `gh pr merge --squash --delete-branch` was used, the remote branch is
already gone — still remove the local worktree and branch.

### 6. Definition of Done = PR MERGED to main

Every delegated task's DoD must **explicitly** list:

- [ ] Changes committed to the working branch
- [ ] (If applicable) Branch pushed to origin
- [ ] (If applicable) Tests + lint pass on the committed tree
- [ ] **PR merged to main — a bead may be closed ONLY when its PR is MERGED
  to main; PR-created means the bead stays `in_progress` with the PR linked.**

A task that says "the code works on disk" but is not in git is **not done**.
Delegation prompts that omit these lines have produced the exact failure mode
this section exists to prevent — include them verbatim.

**"PR created" is not "done."** A bead is `in_progress` from the moment work
starts until the moment its PR lands on `main`. Closing a bead while citing an
open PR number is a Definition-of-Done violation: the work is not merged, may
fail review, may be superseded, and the closed bead now hides real outstanding
work. Until `gh pr view <num> --json state` reports `MERGED`, the bead stays
`in_progress` with the PR number recorded in a comment.

- **Violation pattern (forbidden):** `ce-d2f` was closed citing the still-open
  PR #335. The 2026-06-12 overnight burndown DEAR retro found 25 such beads
  closed against open PRs — every one of them re-surfaced as outstanding work.
- **Correct pattern (required):** `ce-eky` stayed `in_progress` with its PR
  linked, and was closed only after the PR actually merged to main. Follow
  ce-eky, never the ce-d2f violation.

The repo ships a reconciler (`make build-bead-pr-sync` → `bead-pr-sync`) that
cross-references closed beads citing `#NNN` PR refs against live PR state and
reopens any DoD violators. If you find a closed bead whose PR is still open,
that is a defect — reopen the bead and let the reconciler catch the class.

## PR Lifecycle — Wayfinder-Only (MANDATORY)

Raw `gh pr create|close|reopen` in Bash is **denied** in this repo; PR open
and close are allowed only through the `safe-pr` wrapper, which requires a
wayfinder session trace. This is the enforcement tier of principles 5 (use
Wayfinder) and 9 (atomic action wrappers): the PreToolUse hook
`.claude/hooks/pretool-pr-guard` (exit 2, positive guidance) is wired in
`.claude/settings.json`. The hook matches the literal command forms that
cooperative agents type (including `gtimeout`/`env`/`sudo` prefixes and
line continuations) — it is a best-effort net for the forgetful-agent path,
not a security boundary; `gh api` mutations and in-code creators are tracked
separately (see follow-ups below).

**Why:** untraced PRs have no plan/session/bead attribution and no telemetry,
and PR spray from burndown workers has repeatedly outrun the serial merge
pipeline (required checks + linear history), burning CI and bot-review quota
(2026-06-11/12 logjams). Instruction-tier rules alone did not stop it.

**The sanctioned path:**

```
safe-pr create --wayfinder <wayfinder-project-dir> --title "..." --body "..." [gh flags]
safe-pr close  --wayfinder <wayfinder-project-dir> <number|url>
```

- The project dir (or `WAYFINDER_PROJECT_DIR`) must hold a
  `WAYFINDER-STATUS.md` with `status: in_progress`; the session id is stamped
  into the PR body (create) or close comment (close).
- Every invocation appends a JSONL audit record to
  `~/.local/state/dear-agent/safe-pr.log` and emits an OTel span
  (`safepr.<verb>`).
- `make install-safe-pr` installs the binary; `Bash(safe-pr:*)` is
  allow-listed for Claude in `.claude/settings.json` (vetted wrapper, no
  per-call approval). Other harnesses should invoke the same wrapper through
  their native shell/tool permission path.
- **Emergency hatch** (no session exists and the work genuinely cannot wait):
  `safe-pr <verb> --emergency --reason "<why>"` — audited and stamped on the
  PR, never silent. Do not use it to skip starting a wayfinder session.
- Unchanged: read-only `gh pr view|list|checks|diff`, and `gh pr merge`
  (already governed by required checks + review gates). `gh pr reopen` has
  no sanctioned automated path — reopening is a human decision; ask the
  supervisor/user.

Scope today: this repo (project-scoped hook). Global rollout across repos is
ce-20en; routing the in-code PR creators through safe-pr is ce-ijsq
(pkg/selfimprove) and ce-jzqa (agm evaluation); the burndown worker prompt
update is ce-gzmr.

## Stale PR Strategy — safe-rebase (MANDATORY)

When a PR has merge conflicts or is behind main, use `safe-rebase` — the
approved, deterministic merge strategy for agents:

```
safe-rebase -C ~/worktrees/dear-agent/<branch>
safe-rebase -C ~/worktrees/dear-agent/<branch> --auto   # rebase + push + preflight
```

**What it does:**

1. Fetches latest `origin/main`
2. Runs `git rebase origin/main` on the current feature branch
3. **On conflict:** aborts the rebase cleanly, reports the conflicting files,
   and exits non-zero — agents MUST NOT auto-resolve conflicts
4. **On clean rebase:** reports success; with `--auto`, force-pushes
   (`--force-with-lease`) and runs `make preflight`

**Safety invariants:**

- **REFUSES** to operate on protected branches (main, master, develop, release)
- Force-push uses `--force-with-lease`, not `--force` — protects against
  upstream changes by someone else
- Network ops bounded by timeout + `GIT_TERMINAL_PROMPT=0`
- Every operation audit-logged to `~/.local/state/dear-agent/safe-rebase-audit.jsonl`

**When to use `--auto`:** for mechanical PRs where the rebase cannot
introduce semantic conflicts (dependency bumps, docs-only, generated code).
For PRs with logic changes, omit `--auto` and review the rebase result
before pushing.

**Build:** `make build-safe-rebase && make install-safe-rebase`

## Resolving PR Review Threads (MANDATORY)

To resolve GitHub PR review threads (from Gemini, CodeQL, or other bot
reviewers), use `resolve-review-threads` — **never** raw `gh api graphql`
with `resolveReviewThread`. The classifier blocks bare GraphQL mutations,
and the allow rule `Bash(gh api:*)` does not match `gh api graphql ...`
(colon vs space mismatch). The binary wraps the same GraphQL call safely.

```
resolve-review-threads list        <owner> <repo> <pr>           # unresolved threads
resolve-review-threads resolve-all <owner> <repo> <pr> [author]  # resolve all (optional author filter)
resolve-review-threads resolve     <threadId>                    # resolve one
```

Common pattern for bot threads before merge:
```
resolve-review-threads resolve-all vbonnet dear-agent <pr> gemini-code-assist[bot]
```

**Build:** `make build-resolve-review-threads && make install-resolve-review-threads`

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
- `AGENTS.md` — model-agnostic agent instructions that constrain code in this
  repo

**In `~/src/engram-research` (temporal artifacts):**
- Design docs, problem statements, wayfinder artifacts, DEAR retros
- Research, analysis, literature reviews
- Any document whose value is primarily historical or exploratory

See `.dear-agent.yml > forbidden-paths` for the machine-readable enforcement.

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
2. Write a one-paragraph DEAR retro entry in the configured
   research/knowledge-base destination explaining *why* the drift happened and
   what process gap allowed it.
3. File a Beads task if the retro surfaces an action item.

Do **not** silently fix a stale doc without the retro. Silent fixes hide
process gaps that will produce more drift.

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on
confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i`
(interactive) mode on some systems, causing the agent to hang indefinitely
waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:970c3bf2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full
workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
