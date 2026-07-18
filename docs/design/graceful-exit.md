# Graceful Exit — The No-Overfit Guardrail

> Companion to [ADR-018](../adr/ADR-018-graceful-exit-framework-default.md).
> This doc is the callsite cookbook: what the guardrail means in
> practice, when it fires, how to opt out, and how to write a task
> that benefits from it.

## TL;DR

**"Nothing fits" is a first-class valid outcome.** A dear-agent
worker is permitted — and expected — to report an empty result
when the evidence does not support a positive finding, rather than
inflate a weak match to meet an imagined quota.

The guardrail is a framework-level **default**: every task
inherits it, and the worker sees the contract at session start. No
per-prompt opt-in is required. To **opt out** for a specific repo,
set `framework-defaults.graceful-exit.disabled: true` in
`.dear-agent.yml` and document `why:`.

## Why this is a default and not a prompt instruction

Two earlier mitigations failed in practice:

- **Per-prompt instructions** ("if nothing fits, report that")
  protect only the prompt that contains them. A sub-agent spawned
  by that prompt does not inherit the line, and the next sibling
  prompt forgets it entirely.
- **CLAUDE.md guidance** reaches the worker only if the worker
  reads carefully. The same failure mode shows up in the routing
  history captured in `AGENTS.why.md`.

The fix is structural: publish the contract once, at session
start, where every sub-prompt will see it. The worker reads it,
re-reads it on `tmux capture-pane`, and audit transcripts include
it as an artifact. A worker cannot "forget" a banner the way it
can forget a sentence buried in a prompt.

## Where the guardrail fires

The default applies to every task kind. The package exposes an
informational `Applies` list of task kinds where overfit is most
common and the guardrail matters most:

| Task kind         | Failure mode without the guardrail                                    |
|-------------------|-----------------------------------------------------------------------|
| `search`          | Returns a marginal hit rather than "no match in the corpus".          |
| `research`        | Invents weak citations rather than "the literature does not address this". |
| `pattern-match`   | Flags merely-similar code as duplicate / dead.                        |
| `code-review`     | Escalates style nits to "findings" so the report is not empty.        |
| `backfill`        | Synthesises a row when the source yields none.                        |
| `recommendation`  | Surfaces low-confidence suggestions as recommendations.               |

A worker that wants to self-check "is this task one of the
risky kinds?" can read `gracefulexit.Applies()`. The framework
still applies the guardrail to every kind; the list is a hint, not
a gate.

## How a worker is told

When `agm new` creates a session it prints, before any prompt is
sent:

```
Framework guardrail — graceful exit (no-overfit):
  If nothing fits the criteria, report that. "Nothing found" is a
  first-class valid outcome — saying so is better than inflating a
  weak match to meet an imagined quota.
  Applies to: search, research, pattern-match, code-review,
  backfill, recommendation.
  To opt out for this repo, set framework-defaults.graceful-exit.disabled: true
  in .dear-agent.yml (a `why:` is required).
```

The banner is visible to the worker, captured in session
transcripts, and reproducible from `agm acceptance show`-adjacent
tooling. A reviewer auditing a deliverable can point to "the
worker was told this; here is what it produced anyway".

## How a task author opts in to typed criteria

In addition to the framework default, a task can record an
**explicit** acceptance criterion of type `graceful-exit`. This is
useful when:

- The audit phase needs a machine-checkable row that says "yes,
  this task accepts an empty result"; this is what enables a
  future DEAR Audit check to refuse inflated findings.
- The task wants to attach a more specific description (e.g.
  "Empty findings are valid; do not synthesize candidates from
  similarity < 0.6").

```yaml
acceptance-criteria:
  - type: graceful-exit
    description: "Empty findings are a valid completion"
```

Like `no-regressions`, this criterion is **declarative** — it has
no `command:`. The Audit phase carries the check; the Define
phase carries the declaration.

## How a repo opts out

Genuine "always emit a row" tasks exist: a synthetic-data
generator that produces fixtures whether or not real data is
available, an always-on health monitor that must heartbeat. Those
repos can disable the guardrail:

```yaml
framework-defaults:
  graceful-exit:
    disabled: true
    why: "synthetic data generator that must always emit a row"
```

The `why:` field is **required**. The loader rejects an opt-out
without one, both to force a moment of deliberation at the time
the opt-out is added and to leave a breadcrumb for a future
reader who is wondering whether the override is still load-
bearing. The pattern mirrors the
[AGENTS.why.md](../../AGENTS.why.md) tier model: every override
carries its rationale.

## Interaction with handoff confidence

This guardrail is the upstream half of the handoff-confidence
work. The hypothesis: a handoff is only as good as the honesty of
the upstream finding. If agent A inflates a weak match and hands
it to agent B, agent B reads "candidate" as "candidate-with-
context" and acts as if the upstream judgment were calibrated.

By publishing the no-overfit contract at the framework level, we
make the upstream side of every handoff more honest. The
downstream side (calibrated handoff messages, confidence scores)
remains future work — but it can now assume that "no findings" is
a *signal*, not a *failure*.

## Future work — the Enforcement tier

ADR-018 §D5 records the deferred Enforcement tier: an Audit-phase
check that compares findings against the criterion and flags
findings whose evidence-to-conclusion gap exceeds a threshold. It
is deferred because:

- `Finding.Evidence` is still free-form `map[string]any`; a
  typed confidence rubric must come first.
- The two-tier (Instruction + Configuration) precedent for
  output routing held without an Enforcement tier; we expect the
  same here, and want to observe before paying the cost.

If a leak occurs despite the banner and the criterion, escalate by
adding `pkg/audit/checks/gracefulexit.go` and registering it on
`audits.schedule.daily`.

## See also

- [ADR-018](../adr/ADR-018-graceful-exit-framework-default.md) —
  the decision record.
- [AGENTS.why.md](../../AGENTS.why.md) — the instruction +
  configuration precedent.
- `pkg/gracefulexit` — the canonical guardrail text and loader.
- `pkg/acceptance` — the `graceful-exit` criterion type.
