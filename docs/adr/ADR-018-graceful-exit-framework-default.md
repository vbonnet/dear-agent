# ADR-018: Graceful Exit as a Framework Default

Status: Accepted (2026-05-12)

Agents asked to search, research, or pattern-match overshoot when evidence
is weak. A literature review invents marginal citations. A code-review sweep
escalates style nits to "findings" so the report is not empty. A backfill
synthesises a row because the column is `NOT NULL`. A duplicate-detector
flags merely-similar functions so the dashboard does not read zero. Each
case has the same shape — the task is implicitly expected to return
non-empty, the agent inflates a weak match, and the inflated result
propagates as if it were strong.

Per-prompt opt-ins ("report nothing if nothing fits") and CLAUDE.md guidance
both fail: the next prompt forgets, sub-agents do not inherit, and the fix
only holds while someone remembers to apply it. Defaults that depend on
remembering do not hold; copied per-prompt guidance has the same failure mode.

Adopt **graceful exit** ("nothing fits" is a first-class valid outcome) as
a **framework-level default**, two-tier like output routing:

- **Instruction.** `agm new` prints a banner sourced from
  `pkg/gracefulexit.Guardrail` at session start, so every worker — and any
  sub-agent spawned mid-session — sees the contract before composing its
  first response. The text is captured in `tmux` output, so the Audit phase
  has a stable artifact ("the worker was told this; here is the finding it
  produced anyway").
- **Configuration.** A repo whose task genuinely requires a non-empty result
  (a synthetic data generator) opts out under `framework-defaults.graceful-exit`
  in `.dear-agent.yml` and **must** supply `why:`. The validator rejects an
  opt-out without one so exceptions remain auditable.
- **Enforcement is deferred.** A future `gracefulexit` audit check
  ([ADR-011](ADR-011-dear-audit-subsystem.md)) would flag findings with
  too-wide evidence-to-conclusion gaps. We want to see whether
  Instruction + Configuration shift behaviour first; `Finding.Evidence` is
  still `map[string]any` and a confidence rubric needs a typed shape.

Centralising the text in `pkg/gracefulexit` matters: a worker that wants to
assert "I'm held to the no-overfit rule" reads `gracefulexit.Guardrail`. A
drift between banner, criterion, and downstream tools would silently weaken
the contract.

> **Terminology note.** This ADR extends the workflow-engine lifecycle hooks
> (Define / **Enforce** / Audit / **Resolve & Refine**), distinct
> from canonical Process DEAR (Define / **Execute** / Audit /
> **Retro**); see [ADR-035](ADR-035-dear-terminology-disambiguation.md).
