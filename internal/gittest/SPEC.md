# Hermetic Git Test Sandbox Specification

<!-- Last audited at: 2026-07-21 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/gittest`.

## Overview

`internal/gittest` builds the Git subprocesses used by tests that need a real
repository. It exists because the audit of commit
`97a32d415cc858ab8e726c8a66ddb5c8ef10dfac` found that a plain
`go test -count=1 ./...` inherited the developer's global `core.hooksPath`,
ran the real post-merge hook from two temporary repositories, and launched two
repository-wide `agm worktree sweep --execute` processes that deleted two live
worktrees (finding F-01, bead ce-3knl.1).

Isolation is established two ways at once. The environment is rebuilt without
any `GIT_*`, `HOME`, or `XDG_CONFIG_HOME` inherited from the host, and every
invocation carries `-c core.hooksPath=<empty dir>` on the command line.
Command-line configuration outranks every file-based source and Git re-exports
it to the processes it spawns, so neither a repository-local config nor a
Git-spawned child can reintroduce a host hook.

## EARS Requirements

**GITTEST-01** When a test requests a Git sandbox, the system shall root its home directory, hooks directory, template directory, and global configuration file inside a directory owned by that test.

**GITTEST-02** When the system builds the environment for a sandboxed Git subprocess, the system shall drop every inherited `GIT_*`, `HOME`, and `XDG_CONFIG_HOME` variable before adding the sandbox's own values.

**GITTEST-03** When the system invokes Git for a sandbox, the system shall force an empty hooks path through command-line configuration so no host hook is reachable from the invocation or from any process Git spawns.

**GITTEST-04** When a sandboxed repository is created, committed to, branched, and merged, the system shall execute no hook installed by the host's global Git configuration.

**GITTEST-05** When a sandboxed invocation writes global Git configuration, the system shall direct the write to the sandbox's configuration file and leave the host's configuration unchanged.

**GITTEST-06** When package-level helpers are called from one test, the system shall reuse a single sandbox for that test so repositories created by one call remain usable by the next.

**GITTEST-07** When a test file builds a Git command without this package, the system shall fail unless that file carries a reviewed exemption, and shall also fail when an exemption no longer describes a real call site.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- `internal/gittest/gittest_test.go` covers GITTEST-01 through GITTEST-06.
  `TestHostHooksFireWithoutIsolation` is the positive control for GITTEST-04:
  it proves the canary hooks fire for an unisolated repository, so the
  isolation assertion cannot pass vacuously.
- `internal/gittest/guard_test.go` covers GITTEST-07. Isolating today's call
  sites fixes the tests that exist; the guard is what stops the next one from
  reintroducing the hazard.
