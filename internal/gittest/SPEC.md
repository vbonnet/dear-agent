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

Isolation is established three ways at once, because the three cover different
callers:

1. The environment is rebuilt without any `GIT_*`, `HOME`, or
   `XDG_CONFIG_HOME` inherited from the host.
2. Every invocation this package builds carries `-c core.hooksPath=<empty dir>`
   on the command line. Command-line configuration outranks every file-based
   source and Git re-exports it to the processes it spawns.
3. Sandboxed repositories carry the empty hooks path in their own
   configuration. Layers 1 and 2 only reach commands this package builds;
   production Git wrappers build their own `*exec.Cmd` and never set
   `Cmd.Env`, so a test that points one at a temporary repository would
   otherwise re-create the same hazard inside the code under test.

## EARS Requirements

**GITTEST-01** When a test requests a Git sandbox, the system shall root its home directory, hooks directory, template directory, and global configuration file inside a directory owned by that test.

**GITTEST-02** When the system builds the environment for a sandboxed Git subprocess, the system shall drop every inherited `GIT_*`, `HOME`, and `XDG_CONFIG_HOME` variable before adding the sandbox's own values.

**GITTEST-03** When the system invokes Git for a sandbox, the system shall force an empty hooks path through command-line configuration so no host hook is reachable from the invocation or from any process Git spawns.

**GITTEST-04** When a sandboxed repository is created, committed to, branched, and merged, the system shall execute no hook installed by the host's global Git configuration.

**GITTEST-05** When a sandboxed invocation writes global Git configuration, the system shall direct the write to the sandbox's configuration file and leave the host's configuration unchanged.

**GITTEST-06** When package-level helpers are called from one test, the system shall reuse a single sandbox for that test so repositories created by one call remain usable by the next.

**GITTEST-07** When a test file builds a Git command without this package, the system shall fail unless that file carries a reviewed exemption, and shall also fail when an exemption no longer describes a real call site.

**GITTEST-08** When the system initializes a sandboxed repository, the system shall write the empty hooks path into that repository's own configuration, so a Git command built elsewhere and run against it cannot reach a host hook.

**GITTEST-09** When a caller overrides the hooks path on the command line for a sandboxed repository, the system shall allow that caller's own hooks to run.

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
- `TestProductionStyleGitCannotRunHostHooksInSandboxRepos` covers GITTEST-08.
  Env() and Command() only reach commands this package builds; production Git
  wrappers set no `Cmd.Env`, so pointing one at a temporary repository would
  otherwise re-create F-01 inside the code under test. Repository
  configuration outranks global configuration, which is what closes it.
- `TestSandboxRepoStillAllowsItsOwnHooks` covers GITTEST-09 and keeps
  GITTEST-08 from becoming a blanket ban.
