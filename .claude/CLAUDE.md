# dear-agent — Project Instructions

## Core Engineering Principles (MANDATORY)

These eight principles govern *how* work happens in this repo, regardless of
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
   navigable, the toolchain (`make preflight`, lint, vulncheck) uniform, and
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
   See **Dogfooding — Use AGM and VROOM** below for the operational detail.

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
