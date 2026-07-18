# Devlog decisions

Status: Accepted

## Context

Devlog manages several repositories and worktrees from one workspace. It needs
portable Git behavior, team configuration, local overrides, and validation at
the filesystem boundary.

## Decisions

1. **Repository boundary.** Commands depend on the `git.Repository`
   interface; the local implementation shells out to the installed Git CLI.
2. **Bare-repository support.** Bare repositories with named worktrees are a
   first-class layout, alongside standard repositories.
3. **YAML configuration.** A committed base configuration describes the
   workspace. An optional local file extends it additively without replacing
   base repositories.
4. **Validate before mutation.** Configuration loading bounds input size and
   validates names, URLs, paths, duplicates, and worktree relationships before
   Git operations run.

Ordinary Cobra usage and idempotent command implementation are coding patterns,
not separate architecture decisions.

## Consequences

- Git semantics come from the host CLI and are isolated behind one interface.
- Local configuration cannot silently remove team configuration.
- Unsupported or unsafe repository inputs fail before mutation.

## Evidence

- `internal/git/`
- `internal/config/` and their tests
