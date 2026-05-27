# ADR-018: Graceful Exit as a Framework Default

**Status**: Accepted
**Date**: 2026-05-12
**Context**: Extends the workflow-engine *code* DEAR lifecycle (Define,
**Enforce**, Audit, **Resolve & Refine**) captured in
[ADR-011](ADR-011-dear-audit-subsystem.md) — distinct from the **process**
retrospective loop, which was renamed to **DEASR** (Diagnose → Evaluate →
scAle-test → Act → Review) by
[ADR-024](ADR-024-deasr-push-bike-philosophy.md). See
[/CONTEXT.md § Known Terminology Collisions (2b)](../../CONTEXT.md#known-terminology-collisions).
Builds on the two-tier (instruction + configuration) routing model documented
in [AGENTS.why.md](../../AGENTS.why.md).

---

## Context

Agents asked to search, research, or pattern-match systematically
overshoot when the evidence is weak: faced with "find candidates",
they produce candidates whether or not real ones exist. Examples we
have seen in this repo and its predecessors:

- A literature-review pass that invents marginal citations rather
  than reporting that the corpus contains no good match.
- A code-review sweep that escalates style nits to "findings" so the
  report is not empty.
- A backfill ingestion job that synthesises a row when its source
  yields none, because the schema says the column is `NOT NULL`.
- A duplicate-detector that flags merely-similar functions as
  duplicates so the dashboard does not read zero.

Each case has the same shape: the task is implicitly expected to
produce a non-empty result, the agent reads that implicit contract,
and the agent inflates a weak match to meet it. The inflated result
then propagates: a downstream agent reads "candidate" as
"high-confidence candidate" and treats it as such. This compounds
across handoffs — the very failure mode the wayfinder /
handoff-confidence work was created to prevent.

Two earlier mitigations did not stick:

1. **Per-prompt opt-in.** Users added "report nothing if nothing
   fits" to individual prompts. This works for the specific prompt,
   but the next prompt forgets, and any sub-agent spawned by the
   first does not inherit the instruction. The fix only holds while
   someone remembers to apply it.

2. **CLAUDE.md guidance.** A line buried in `.claude/CLAUDE.md`
   reaches the worker only if the worker reads it carefully. The
   AGENTS.why.md routing precedent shows the same failure mode:
   research artifacts leaked into the code repo twice while the
   guidance was instruction-only, and only stopped after the
   *configuration* tier was added.

The pattern is clear: defaults that depend on remembering do not hold.

---

## Decision

Adopt **graceful exit** ("nothing fits" is a first-class valid
outcome) as a **framework-level default** in dear-agent. The default
applies to every task; opt-out is explicit, repo-scoped, and requires
a documented `why:`.

This is the same two-tier shape as output routing: **Instruction**
(the CLAUDE.md / banner sentence) + **Configuration** (a single
authoritative entry in `.dear-agent.yml`). A third **Enforcement**
tier — a DEAR Audit check that flags inflated findings — is
deferred.

### D1. Single source of truth: `pkg/gracefulexit`

The canonical guardrail text, the list of task kinds it applies to,
and the `.dear-agent.yml` config loader all live in
`pkg/gracefulexit`. Three call sites consume it:

- `agm new` prints the banner at session start so every worker (and
  any sub-agent it spawns) sees the contract before composing its
  first response.
- `pkg/acceptance` recognises `graceful-exit` as a typed criterion
  so the DEASR **Diagnose** phase can record it alongside `tests-pass` and
  `lint-clean`. This is what gives the Scale-test / Act phases a hook later.
- Future: a `gracefulexit` audit check (see deferred Enforcement
  below) can compare findings against the criterion.

Centralising the text matters because the guardrail is
wire-readable: a worker that wants to assert "I am being held to the
no-overfit rule" reads `gracefulexit.Guardrail`. A drift in wording
between the banner, the criterion, and downstream tools would
silently weaken the contract.

### D2. Default on, explicit opt-out

The zero-value `Config` is enabled. Repos that genuinely require a
non-empty result (e.g. a synthetic data generator) set:

```yaml
framework-defaults:
  graceful-exit:
    disabled: true
    why: "synthetic data generator that must always emit a row"
```

The `why:` field is **required** when `disabled: true`. The
validator rejects an opt-out without one. This mirrors the
AGENTS.why.md philosophy: every override carries its rationale, so a
future reader can tell whether the override is still load-bearing.

### D3. Travels with the session, not the prompt

The banner prints during `agm new`, before the worker reads its
first prompt. That placement is load-bearing:

- A sub-agent spawned mid-session inherits the same tmux session and
  therefore the same banner; it does not need to re-read CLAUDE.md
  or be re-instructed.
- The banner is visible in `tmux capture-pane` output and in any
  AGM-captured session transcript, so the Audit phase has a stable
  artifact to point to ("the worker was told this; here is the
  finding it produced anyway").
- A user who *forgets* to mention the guardrail in their prompt does
  not lose the protection.

This is why we did not implement the rule as an MCP tool or a
per-prompt template: those surfaces are opt-in per call site. A
banner at session start is opt-out per repo.

### D4. Applies to all task kinds, with an informational catalog

The guardrail is universal — every dear-agent task inherits it. The
package exposes an `Applies` slice (`search`, `research`,
`pattern-match`, `code-review`, `backfill`, `recommendation`) as
informational. A worker that wants to self-check "is this task
especially at risk?" can consult the list; the framework still
applies the guardrail whether or not the task kind is named.

### D5. Deferred: an Audit-phase enforcement check

A `gracefulexit` audit check would consume the finding stream from
ADR-011 §D3 and flag findings whose evidence-to-conclusion gap
exceeds a threshold (e.g. "candidate" claims with a single
low-similarity match). It is deferred because:

- The evidence model in `Finding.Evidence` is still
  free-form `map[string]any`; a confidence rubric needs a typed
  shape first.
- We want to observe whether the Instruction + Configuration tiers
  alone shift agent behaviour, before paying the cost of an
  enforcement tier. This is the same staging decision recorded in
  AGENTS.why.md for output routing — and that decision held.

If a leak occurs despite the default banner and the criterion, the
escalation is to add `audit/checks/gracefulexit.go` and register it
with `audits.schedule.daily`.

---

## Consequences

### Positive

- A worker entering a search, research, or pattern-match task in
  dear-agent inherits the no-overfit contract automatically; no
  per-prompt remembering required.
- The contract is **typed** (the `TypeGracefulExit` criterion) and
  **textual** (the banner), so both machine-readable audits and
  human review can refer to the same artifact.
- Sub-agents and prompt chains inherit the contract via the
  session, not via prompt-level propagation.
- Adding a new framework default in future is cheap: a sibling
  section under `framework-defaults` and a corresponding package.

### Negative

- The banner adds two-to-four lines to session start output. This
  is the price of moving the contract from "buried in a doc" to
  "visible to every worker". Acceptable.
- A genuinely non-empty-result task (synthetic data generator,
  always-on monitor) must explicitly opt out. We expect this to be
  rare; the friction is intentional, and the `why:` field captures
  why an opt-out was added.
- The acceptance package now has one more criterion type, which
  must stay in sync with the gracefulexit package. The relationship
  is one-way (acceptance imports nothing from gracefulexit, only
  declares a parallel type) so the coupling is annotation-only.

### Neutral

- No change to existing acceptance-criteria entries: the new type
  is additive and the validator is unchanged for existing rows.
- No CI gate yet — the Audit-tier enforcement is deferred until the
  Instruction + Configuration tiers have been observed to fail.

---

## Implementation

- `pkg/gracefulexit` — guardrail text, config loader, banner.
- `pkg/acceptance` — new `TypeGracefulExit` criterion type
  (additive; existing rows unaffected).
- `agm/cmd/agm/acceptance.go` — `announceFrameworkGuardrails`
  helper.
- `agm/cmd/agm/new.go` — calls the helper alongside
  `announceAcceptanceCriteria` during `agm new`.
- `.dear-agent.yml` — documents the `framework-defaults` section
  and inherits the on-by-default behaviour.

---

## References

- [ADR-011: DEAR Audit Subsystem](ADR-011-dear-audit-subsystem.md) —
  where the deferred Enforcement-tier audit check would live.
- [AGENTS.why.md](../../AGENTS.why.md) — the two-tier (instruction +
  configuration) precedent.
- [docs/design/graceful-exit.md](../design/graceful-exit.md) —
  callsite cookbook: how a research / search / pattern-match author
  uses the guardrail in practice.
