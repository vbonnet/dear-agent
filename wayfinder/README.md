# Wayfinder

Wayfinder is a Go CLI for a gated nine-phase development workflow:

`CHARTER → PROBLEM → RESEARCH → DESIGN → SPEC → PLAN → SETUP → BUILD → RETRO`

The executable interface is `wayfinder session`; session state lives in the
YAML frontmatter of `WAYFINDER-STATUS.md`.

Install the executable before installing or invoking the companion agent skill:

```sh
go install github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder@latest
```

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
wayfinder -C <project-dir> session complete-phase PROBLEM --outcome success
```

Run `wayfinder session --help` for the current command surface. Do not edit
the status file manually or rely on retired phase identifiers.

## Source of truth

- [SKILL.md](SKILL.md): compact agent workflow
- [PHASES.md](PHASES.md): phase intent and artifact names
- [ARCHITECTURE.md](ARCHITECTURE.md): current implementation map
- [SPEC.md](SPEC.md): observable requirements
