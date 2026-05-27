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
  `docs/retros/` if the change exposes a process gap. **dear-agent layers
  four MANDATORY project-specific additions** on top of the standard DEAR
  shape (Scaling-model block, ideal-first solution, Scale-test scoring,
  rip-out tax `.why.md`) — see the template at
  [docs/retros/_TEMPLATE.md](../docs/retros/_TEMPLATE.md),
  [ADR-024](../docs/adr/ADR-024-push-bike-and-dear-retro-extensions.md),
  and the "Push-bike, not training wheels" section below.

**Why this is a rule, not a suggestion:** dogfooding surfaces real gaps
before users hit them. Every time we route around our own tools, we lose a
data point and silently widen the gap between "what we ship" and "what we
trust." If a tool is too painful to use on its own repo, that pain is a bug
to file (or fix), not a reason to bypass.

**Acceptable bypass:** trivial single-file edits, one-shot reads, and the
literal bootstrap case where the tool itself is broken (in which case: file
an issue or write a retro before moving on).

## Push-bike, not training wheels (MANDATORY design constraint)

This is **dear-agent's** project-level design constraint for every DEAR
retro and every non-trivial change in this repo. It is **not** part of
DEAR itself — DEAR's four-letter loop stays unchanged. Rationale,
mechanism, and the per-project extension model are in
[ADR-024](../docs/adr/ADR-024-push-bike-and-dear-retro-extensions.md).

Prefer fixes that teach the system the right reflex at target scale over
bolted-on caps that have to be removed before the system can ride for real.

**A push-bike** is the simplest shape of the *eventual* bike — frame,
wheels, no pedals. Everything learned on it (balance, steering, momentum)
transfers. Nothing has to be ripped out when pedals arrive.

**Training wheels** are bolted-on caps that prevent the rider from
learning the thing that actually matters. They buy time at the cost of
teaching the wrong reflex.

**When working on this repo (dear-agent's four MANDATORY additions to its
DEAR retros):**

1. **Define → Scaling model.** State current scale, target scale, scaling
   model (linear / sub-linear / super-linear in what dimension), and the
   binding constraint at target. Unknowns get "unknown — needs
   measurement" and a Retro action item, not deletion.
2. **Execute → Ideal-first solution.** Describe the ideal scalable
   solution first, with no reference to current code or this-week
   constraints. Then scope down to the minimum on the path to it. Reject
   any minimum that has to be ripped out later.
3. **Audit → Scale-test.** Score every proposed fix *scales* / *neutral*
   / *caps* on 10× agents, 10× machines, 10× users. A `caps` score on any
   axis is allowed *only* when the fix carries a co-located `.why.md`
   declaring the **rip-out tax** — removal cost, code touched, removal
   trigger.
4. **Cite "push-bike, not training wheels"** as a legitimate veto when
   reviewing a proposed fix. It forces the proposer to either reframe the
   fix or attach a rip-out tax.

See [/CONTEXT.md § Push-bike, not training wheels](../CONTEXT.md#push-bike-not-training-wheels--design-constraint-for-dear-retros-dear-agent),
[/CONTEXT.md § dear-agent's additions to the standard DEAR retro](../CONTEXT.md#dear-agents-additions-to-the-standard-dear-retro),
[ADR-024](../docs/adr/ADR-024-push-bike-and-dear-retro-extensions.md), and
[AGENTS.why.md](../AGENTS.why.md) for the rip-out-tax `.why.md` pattern.
