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
   validates URLs, explicit paths, duplicates, and worktree relationships before
   Git operations run. Repository names are required but are trusted
   configuration; they are not currently a path-containment boundary.

Ordinary Cobra usage and idempotent command implementation are coding patterns,
not separate architecture decisions.

## Consequences

- Git semantics come from the host CLI and are isolated behind one interface.
- Local configuration cannot silently remove team configuration.
- Invalid governed URL, path, duplicate, or worktree relationships fail before
  mutation; repository names must come from a trusted configuration source.

## Evidence

- `internal/git/`
- `internal/config/` and their tests
