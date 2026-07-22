# Git Hook Lifecycle Specification

<!-- Last audited at: 2026-07-22 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Repository-level git hooks under `scripts/git-hooks/`.

## Overview

`scripts/git-hooks` owns repository lifecycle hooks that run outside any single
agent harness. The global `post-merge` hook keeps local runtime artifacts aligned
with trunk after a default-branch pull or merge while remaining inert in foreign
repositories and feature-branch integration merges. It is intentionally fail-safe:
maintenance work may warn, skip, or log, but it must not block the git operation
that triggered it.

## EARS Requirements

**GITHOOK-01** When a post-merge hook runs outside the repository default branch, the system shall exit without rebuilding binaries, deploying host artifacts, transitioning beads, or sweeping worktrees.

**GITHOOK-02** When a post-merge hook runs in a repository that does not expose a dear-agent target surface, the system shall treat that repository as out of scope.

**GITHOOK-03** When a default-branch merge changes build-relevant source for a managed Go binary, the system shall rebuild only the affected binary targets.

**GITHOOK-04** When a default-branch merge changes only docs, tests, or unrelated configuration, the system shall skip binary rebuilds.

**GITHOOK-05** When rebuilding a managed Go binary after a merge, the system shall build from freshly resolved `origin/<default_branch>` when that trunk ref is available.

**GITHOOK-06** When no origin trunk ref is available during a rebuild, the system shall fall back to building the local working tree.

**GITHOOK-07** When installing a rebuilt Go binary, the system shall build to a temporary file and atomically rename that file over the target binary.

**GITHOOK-08** When host-artifact deployment is enabled and `deploy/manifest.yaml` exists, the system shall run the manifest-backed `dear-deploy-sync` path from the trunk build context.

**GITHOOK-09** When post-merge deployment verification is enabled and `agm` is installed, the system shall verify that the installed `agm` binary is built from `origin/main` ancestry.

**GITHOOK-10** When a merged commit message contains an explicit GitHub-style closing keyword for a Beads id, the system shall close that bead in the configured Beads store.

**GITHOOK-11** When a merged commit message mentions a Beads id without a closing keyword, the system shall not close that bead.

**GITHOOK-12** When a referenced bead is absent or already closed, the system shall skip that bead without failing the git operation.

**GITHOOK-13** When post-merge worktree sweeping is enabled and `agm` is installed, the system shall invoke `agm worktree sweep --execute` with the hook git environment cleared.

**GITHOOK-14** When any post-merge maintenance stage fails, the system shall preserve a zero exit status for the git operation.

**GITHOOK-15** When a post-merge maintenance stage has an opt-out environment variable set to `0`, the system shall skip that stage.

**GITHOOK-16** When build-relevant AGM source changes, the system shall serialize deployment with a kernel-released lock across checkouts sharing the install directory, use each platform lock tool's file-and-command form, safely recover ownerless or malformed legacy lock directories, make every contender reacquire the lock and refresh trunk, stage both `agm` and the separately installed `agm-reaper` from the same resolved revision, preserve the installed pair if either build fails, and activate the pair only after both builds succeed.

**GITHOOK-17** When build-relevant Wayfinder source changes on the default branch, the system shall serialize activation across checkouts sharing the install directory using the platform lock tool's file-and-command form, make every contender acquire the shared lock before refreshing canonical trunk, and atomically install the Wayfinder binary from that freshly resolved revision.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`

## Test Traceability

- Unit package: `tests/githooks`
