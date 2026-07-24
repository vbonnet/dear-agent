# Wayfinder

Wayfinder is a Go CLI for a gated nine-phase development workflow:

`CHARTER → PROBLEM → RESEARCH → DESIGN → SPEC → PLAN → SETUP → BUILD → RETRO`

The executable interface is `wayfinder session`; session state lives in the
YAML frontmatter of `WAYFINDER-STATUS.md`.

Install the executable before installing or invoking the companion agent skill:

```sh
go install github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder@latest
```

All phase validators are compiled into this executable; no separate review
scripts, Python environment, or Engram checkout is required.

## Quick start

```sh
wayfinder -C <project-dir> session start <project-name> \
  --project-type feature --risk-level M
wayfinder -C <project-dir> session status
wayfinder -C <project-dir> session start-phase CHARTER
printf '# Charter\n\n## Objective\nDefine the user-visible outcome.\n\n## Scope\nName included work and explicit exclusions.\n\n## Constraints\nRecord safety, compatibility, and delivery constraints.\n\n## Success\nState measurable acceptance conditions.\n' \
  > <project-dir>/CHARTER-charter.md
wayfinder -C <project-dir> session complete-phase CHARTER --outcome success
wayfinder -C <project-dir> session next-phase
wayfinder -C <project-dir> session start-phase PROBLEM
printf '# Problem Statement\n\n## Current behavior\nDescribe the observable user problem and who experiences it.\n\n## Desired outcome\nState the behavior that should replace it.\n\n## Evidence\nRecord the source, reproduction, or constraint that makes this problem real.\n' \
  > <project-dir>/PROBLEM-statement.md
wayfinder -C <project-dir> session complete-phase PROBLEM --outcome success
```

In Git repositories, phase completion commits its canonical marker files and
`<PHASE>-*.md` artifacts as a scoped commit. Rewinds similarly commit their
status, history, and retrospective updates before the target is restarted.

Run `wayfinder session --help` for the current command surface. Do not edit
the status file manually or rely on retired phase identifiers.

## Source of truth

- [SKILL.md](SKILL.md): compact agent workflow
- [PHASES.md](PHASES.md): phase intent and artifact names
- [ARCHITECTURE.md](ARCHITECTURE.md): current implementation map
- [SPEC.md](SPEC.md): observable requirements

## Related: research-pipeline

`research-pipeline` (top-level `research-pipeline/` plugin) is a separate,
standalone skill for turning an external source (a talk, article, paper) into
verified, executable work. It is not a Wayfinder phase and Wayfinder's own
RESEARCH phase does not delegate to it — Wayfinder's RESEARCH means "search
existing solutions for an already-charter'd project," a narrower thing than
research-pipeline's source-ingestion stages. The one real overlap is
decomposition: `research-pipeline`'s Stage 4 routes large, phased
decompositions through this PLAN phase's Beads adapter rather than
reimplementing bead filing. See
[the incorporate-vs-delegate assessment](https://github.com/vbonnet/dear-agent/pull/947)
for the full reasoning.
