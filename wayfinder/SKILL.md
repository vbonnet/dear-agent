---
name: wayfinder
description: Use when a change needs explicit discovery, design, requirements, planning, implementation evidence, and a retrospective across the nine-phase Wayfinder workflow.
allowed-tools:
  - Bash
  - Read
  - Write
  - Edit
  - Glob
  - Grep
---

# Wayfinder

Wayfinder is a thin interface to the `wayfinder session` CLI. The CLI and
`WAYFINDER-STATUS.md` own transition state; this skill owns only the working
method.

Use Wayfinder for multi-phase or high-risk work where discovery and review
evidence matter. Skip it for an obvious, contained change that can be verified
directly.

## Start

1. Choose the project directory that will contain the status and phase
   artifacts.
2. Inspect the installed interface:

   ```sh
   wayfinder session --help
   wayfinder session start --help
   ```

3. Start the session:

   ```sh
   wayfinder -C <project-dir> session start <project-name> \
     --project-type <feature|research|infrastructure|refactor|bugfix> \
     --risk-level <XS|S|M|L|XL>
   ```

Use `--skip-roadmap` only when SETUP adds no useful task breakdown. Never
edit `WAYFINDER-STATUS.md` by hand.

## Work one phase at a time

The only phase sequence is:

`CHARTER → PROBLEM → RESEARCH → DESIGN → SPEC → PLAN → SETUP → BUILD → RETRO`

For each phase:

```sh
wayfinder -C <project-dir> session status
wayfinder -C <project-dir> session next-phase
wayfinder -C <project-dir> session start-phase <PHASE>
# Create or update <PHASE>-<descriptive-name>.md with truthful evidence.
wayfinder -C <project-dir> session complete-phase <PHASE> --outcome success
```

In a Git repository, `complete-phase` commits the canonical status, history,
and phase Markdown artifacts as one scoped commit. It preserves unrelated
staged and unstaged changes.

Do not bypass a failed gate. Read the error, repair the artifact or
implementation, rerun the relevant verification, and complete the phase again.
Use `--reason` only for the explicit hash-mismatch override described by
`complete-phase --help`.

## Phase intent

| Phase | Required outcome |
|---|---|
| CHARTER | Objective, scope, constraints, success conditions |
| PROBLEM | Evidence that the problem and affected users are understood |
| RESEARCH | Existing-solution search and build/adapt/reuse decision |
| DESIGN | Current architecture, seams, invariants, and trade-offs in both `DESIGN-<name>.md` and `ARCHITECTURE.md` |
| SPEC | Observable requirements and acceptance criteria |
| PLAN | Ordered implementation and verification steps |
| SETUP | Ready workspace and task breakdown, unless explicitly skipped |
| BUILD | Real changes plus reproducible test, review, and delivery evidence |
| RETRO | Outcomes, deviations, lessons, and remaining work |

`RESEARCH-existing-solutions.md`, `ARCHITECTURE.md`,
`SPEC-solution-requirements.md`, `PLAN-design.md`, and `SETUP-plan.md` have
additional deterministic checks.
Use `wayfinder session complete-phase --help` and the returned errors as the
authoritative gate contract.

## Rewind and close

When new evidence invalidates earlier work, rewind explicitly and record why:

```sh
wayfinder -C <project-dir> session rewind-to <PHASE> --reason "<reason>"
```

In a Git repository, `rewind-to` commits its status, history, and retrospective
updates before returning, so the target phase can be started immediately.

End only after the requested outcome is implemented and its required delivery
state is verified:

```sh
wayfinder -C <project-dir> session end --status completed
```

If the requested outcome includes a PR, deployment, or runtime check, local
tests alone are not completion. Record the actual remaining blocker instead of
claiming success.

This workflow is harness-neutral. When skill activation is unavailable, use
the same CLI commands and artifacts directly.

## References

- Phase contract: `PHASES.md`
- Current architecture: `ARCHITECTURE.md`
- Observable requirements: `SPEC.md`
