# ADR-035: DEAR Terminology Disambiguation

Status: Accepted (2026-07-01)

## Context

The repository historically used "DEAR" for two related but different loops:

- **Process DEAR**: Define / Execute / Audit / Retro. This is the
  governance and retrospective loop used by `AGENTS.md`, VROOM process
  guidance, and the policy document
  [`docs/policies/dear-retro.ai.md`](../policies/dear-retro.ai.md).
- **Workflow lifecycle hooks**: Define / Enforce / Audit / Resolve & Refine.
  This is the workflow-engine extension surface exposed as
  `pkg/workflow.Hooks` (`OnDefine`, `OnEnforce`, `OnAudit`, `OnResolve`) and
  described by ADR-010 and ADR-011.

Using the same acronym for both made docs read as if a process retrospective
and a workflow-engine callback lifecycle were the same control loop. They are
not. They share the Define and Audit words, but the E and R phases have
different meanings and different owners.

## Decision

Use a two-level model with explicit names:

| Name | Expansion | Scope | Owner |
|---|---|---|---|
| **Process DEAR** | Define / Execute / Audit / Retro | Per-task governance and retrospectives | AGENTS.md, VROOM, policy docs |
| **Workflow lifecycle hooks** | Define / Enforce / Audit / Resolve & Refine | Workflow-engine callback API | `pkg/workflow`, ADR-010, ADR-011 |

Rules:

1. Bare **DEAR** means **Process DEAR** unless a document is quoting a
   historical filename, issue ID, branch name, or backlog identifier.
2. Workflow-engine docs and comments must use **workflow lifecycle hooks** or
   **Define/Enforce/Audit/Resolve hooks**, not "DEAR hooks".
3. Exported Go names in `pkg/workflow.Hooks` remain unchanged. They do not
   contain the string `DEAR`; renaming `OnDefine`, `OnEnforce`, `OnAudit`, or
   `OnResolve` would be an API break and is not part of this ADR.
4. `DEAR-X.*` backlog IDs are historical identifiers only. They do not define
   either lifecycle.
5. The process-level policy source is
   [`docs/policies/dear-retro.ai.md`](../policies/dear-retro.ai.md). This ADR
   owns only the terminology disambiguation and must not be copied into a
   competing policy file.

## Consequences

- `CONTEXT.md` remains the glossary, but links here for the canonical
  disambiguation.
- ADR-010, ADR-011, workflow-engine docs, and Go comments should describe the
  code surface as workflow lifecycle hooks.
- Reader-facing summaries such as `llms.txt` should present Process DEAR as
  the repo philosophy and mention workflow lifecycle hooks separately only
  when describing the workflow engine.

## Cross-references

- [ADR-010: Workflow Engine Architecture](ADR-010-workflow-engine-architecture.md)
- [ADR-011: Scheduled Repository Audit Subsystem](ADR-011-dear-audit-subsystem.md)
- [/CONTEXT.md](../../CONTEXT.md) § DEAR and § Known Terminology Collisions
- [`docs/policies/dear-retro.ai.md`](../policies/dear-retro.ai.md) — Process DEAR policy.
