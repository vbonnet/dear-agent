# Outcomes framework — goal-driven verification on existing primitives

**Status:** Design (not implemented; one slice landed as `spec.coverage`)
**Last updated:** 2026-05-09
**Owner:** vbonnet

This document defines an *Outcomes* layer that sits above existing
dear-agent verification primitives. There is no new runtime: an
Outcome is a thin declarative wrapper over `acceptance-criteria`,
`exit_gate`, `audit.Check`, and the role registry. The framework is
the agreement that those four surfaces share a common shape so a
single goal ("the SPEC is honored", "models behave consistently")
decomposes into criteria each surface can evaluate.

## Why this exists

Two recurring questions surface in dear-agent sessions:

1. **Spec compliance** — does what we built actually match the
   `SPEC.md`?
2. **Cross-model parity** — does the harness behave consistently
   across Claude / Gemini / Codex / OpenCode?

Both are *outcomes* — properties we want to be true continuously. The
project already has the primitives needed to evaluate them; what's
missing is a shared vocabulary and a thin index so a goal authored
once gets evaluated by whichever surface fits its evaluation cost.

Claude Code itself does not ship a `/goal` or `/outcomes` feature
(verified 2026-05-09 against the CLI flags, skill list, and plugin
marketplaces). The framework here is dear-agent's own answer.

## Primitives we already have

| Primitive | Where it lives | What it evaluates | When it runs |
|---|---|---|---|
| `acceptance-criteria` | `.dear-agent.yml` → [`pkg/acceptance`](../../pkg/acceptance/acceptance.go) | Define-phase exit conditions for a task | Worker reads at start; Audit verifies after |
| `exit_gate` | workflow node body → [`pkg/workflow/exit_gate.go`](../../pkg/workflow/exit_gate.go) | Per-node "is this attempt good enough?" | Each runner attempt, before transitioning to `succeeded` |
| `audit.Check` | [`pkg/audit/checks/`](../../pkg/audit/checks/) | Repo-wide invariants, on a cadence | Scheduled (daily/weekly/monthly) or on demand via `workflow-audit run` |
| Role registry | [`pkg/workflow/roles/`](../../pkg/workflow/roles/) | Resolves a role → primary/secondary/tertiary model tier | Per-node, when an AI executor needs a model |
| `outputs[].durability` | [`pkg/workflow/outputs.go`](../../pkg/workflow/outputs.go) | What persists, where | Runner materializes after a node succeeds |

Together these form a substrate: every "is the goal met?" question can
land on one of them. The framework names that mapping explicitly.

## What an Outcome is

An Outcome is a (name, evaluator, cadence) triple. The evaluator is
**not** new code — it is one of the four primitives above plus a
binding declaring which one applies and how often.

```yaml
outcomes:
  - name: spec-coverage
    evaluator: audit
    check: spec.coverage              # registered in pkg/audit
    cadence: weekly
    severity_ceiling: P2

  - name: tests-pass
    evaluator: acceptance
    type: tests-pass                  # already lives in pkg/acceptance
    command: "go test ./..."

  - name: cross-model-research-parity
    evaluator: workflow                # post-MVS, see §"Cross-model parity"
    workflow: bench/research-parity.yaml
    role: research
    tiers: [primary, secondary, tertiary]
    exit_gate:
      - { kind: confidence_score, target: outputs.summary, min: 0.7 }
```

Resolution rule: `evaluator` is closed-set (`acceptance`, `exit_gate`,
`audit`, `workflow`); the rest of the row is whatever the chosen
evaluator already accepts. The framework is mostly index, very little
code: a loader, a validator, and a `dear-agent outcomes list` /
`outcomes status` CLI that queries the right backend per row.

## Two concrete questions, decomposed

### Spec compliance

> *"Does what we built actually match the SPEC?"*

Decomposes into three checks of increasing depth:

1. **Coverage** — every package directory under `pkg/`, `internal/`,
   `tools/`, `cmd/` has a sibling `SPEC.md`. *Evaluator:* `audit`,
   check `spec.coverage`. **Shipped in this PR.**
2. **Staleness** — the `SPEC.md` last-modified timestamp is not
   substantially older than the directory's most-recent commit.
   *Evaluator:* `audit`, check `spec.staleness` (proposed; backlog).
3. **Conformance** — identifiers / file paths referenced in `SPEC.md`
   resolve in the codebase. *Evaluator:* `audit`, check
   `spec.conformance` (proposed; backlog).

Tier 1 is mechanical and cheap; tier 3 is expensive enough to belong
in the monthly cadence and possibly behind an LLM. The framework's
job is to keep them on a single conceptual axis so an operator who
cares about "spec compliance" gets all three signals from one
configuration block.

### Cross-model parity

> *"Does the harness behave consistently across Claude / Gemini / etc.?"*

The role registry (Phase 1) already resolves a role to a primary /
secondary / tertiary tier across providers. Parity verification is
therefore a workflow that:

1. Picks a canary task per role (e.g., research a 5-line question;
   implement a known-easy ticket; review a known-flawed diff).
2. Runs the task once per tier. Tiers are interchangeable backends
   already — the role registry's reason to exist.
3. Evaluates each output via the same `exit_gate` (e.g.,
   `confidence_score >= 0.7`, `schema_validation` against a known
   answer, or `bash` checking a deterministic property).
4. Records pass/fail per (role, tier) so a regression in any tier
   surfaces as a Finding.

This is exactly the shape of [Phase 6.7 — A/B model testing per DEAR
phase](../../ROADMAP.md#phase-6--mozillamythos-insights). The Outcomes
framework reframes that ticket from "benchmark harness" to "a parity
outcome that happens to use the workflow runner as its evaluator."
The skeleton already lives in `internal/benchmark`; promoting it
finishes the cross-model parity slice.

## Mapping to substrate properties

The substrate hypothesis (ADR-009) requires every component to
answer five diagnostic questions: durability, ownership, state
machine, audit, permissions. Outcomes inherit these from the
evaluator:

| Outcome evaluator | Durability | Ownership | State | Audit | Permissions |
|---|---|---|---|---|---|
| `acceptance` | `.dear-agent.yml` (git_committed) | repo CODEOWNERS | implicit pass/fail | worker-side log | n/a |
| `exit_gate` | `node_outputs` table | `node.role` | `pending → running → succeeded/failed` | `audit_events` row per gate eval | inherits from node `permissions` |
| `audit.Check` | `audit_findings` table | `check.id` | `open → acknowledged → resolved → reopened` | `audit_runs` row | check declares `RequiresNetwork` |
| `workflow` | full SQLite state (Phase 0) | role registry | full DAG state machine | `audit_events` per node | per-node `permissions` |

An Outcome therefore inherits substrate-quality recording for free —
an Outcome failing produces a queryable row, not a log line.

## What this PR ships

1. This design doc (no runtime impact).
2. A new `audit.Check` `spec.coverage` ([pkg/audit/checks/spec_coverage.go](../../pkg/audit/checks/spec_coverage.go))
   that emits one P3 finding per package directory under tracked
   roots that lacks a `SPEC.md`. Default roots: `pkg/`, `internal/`,
   `tools/`, `cmd/`. Configurable via the standard
   `audits.schedule.<cadence>.config` block.

The check is intentionally tier-1 — mechanical, cheap, and a clean
template for the staleness / conformance checks that follow.

## What this PR does not ship

- An `outcomes:` block in `.dear-agent.yml`, an `outcomes` CLI, or a
  loader. Those land when at least two evaluators have a non-trivial
  outcome that benefits from being indexed in one place.
- `spec.staleness` (tier 2) or `spec.conformance` (tier 3). Tracked
  in the [BACKLOG](../workflow-engine/BACKLOG.md) under Phase 6.
- The cross-model parity workflow. Tracked as Phase 6.7.

## Out-of-scope explicitly

- Generating SPECs. dear-agent has `create-spec` skills already; the
  Outcomes framework is verification, not authoring.

## References

- [acceptance-criteria loader](../../pkg/acceptance/acceptance.go)
- [exit_gate evaluator](../../pkg/workflow/exit_gate.go)
- [audit subsystem](../../pkg/audit/) and [ADR-011](../adrs/ADR-011-dear-audit-subsystem.md)
- [role registry](../../pkg/workflow/roles/)
- [ROADMAP — Phase 6](../../ROADMAP.md#phase-6--mozillamythos-insights)
