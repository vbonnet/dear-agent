# dear-agent — Project Instructions

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
| Source code, ADRs (`docs/adrs/`), design docs (`docs/design/`) | this repo                |
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
  `agm new` / `agm send` instead of opening ad-hoc terminals; use
  `agm acceptance show` at the start of a task and check
  `agm admin doctor` if something looks off.
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
