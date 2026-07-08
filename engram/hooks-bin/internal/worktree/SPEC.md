# Engram Hook Worktree Isolation Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/hooks-bin/internal/worktree` provides session-scoped worktree isolation
for hook-driven multi-agent collaboration. It detects repository layout,
formats deterministic session worktree names, provisions worktrees
idempotently, caches provisioned paths, and redirects file paths into the
session worktree when needed.

## EARS Requirements

**EHW-01** When repository detection finds a `.bare` directory, the system shall classify the repository as bare structure.

**EHW-02** When repository detection does not find a `.bare` directory, the system shall classify the repository as standard structure.

**EHW-03** When a standard repository worktree base is requested, the system shall use the expanded `~/worktrees` location.

**EHW-04** When a bare repository worktree base is requested, the system shall use `<repo>/.bare/worktrees`.

**EHW-05** When a provisioner has a cached session path, the system shall return it without invoking git.

**EHW-06** When the expected worktree directory already exists, the system shall cache and return that path without creating a duplicate worktree.

**EHW-07** When a session worktree is missing, the system shall create the parent directory and invoke `git -C <repo> worktree add <path> -b <branch>`.

**EHW-08** When no branch name is configured, the system shall derive the branch name from the formatted session ID.

**EHW-09** When worktree creation fails, the system shall return an error that includes git output.

**EHW-10** When a session path is requested, the system shall derive it deterministically from the worktree base and session ID.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/worktree/*_test.go`

