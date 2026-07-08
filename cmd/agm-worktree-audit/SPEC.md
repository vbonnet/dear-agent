# agm-worktree-audit Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/agm-worktree-audit`.

## Overview

`agm-worktree-audit` is a read-only diagnostic command for finding reclaimable
git worktrees and local branches under a source root. It reports abandoned
worktrees, local-only worktree branches, merged local branches that remain
present, and stale unmerged local branches. Cleanup is intentionally left to
separate tools so the audit path cannot delete work by accident.

## EARS Requirements

**AGM-WORKTREE-AUDIT-01** When the command scans a root directory, the system shall consider only immediate child directories that are git worktrees and skip dot-prefixed directories.

**AGM-WORKTREE-AUDIT-02** When no git repositories are found under the root, the system shall exit successfully after reporting that no repositories were found.

**AGM-WORKTREE-AUDIT-03** When a non-main worktree head is older than the worktree staleness threshold, the system shall report an abandoned-worktree finding.

**AGM-WORKTREE-AUDIT-04** When a non-main worktree branch has no matching origin branch, the system shall report a worktree-no-remote finding.

**AGM-WORKTREE-AUDIT-05** When a local branch is already merged into the resolved base ref and is not the base branch, the system shall report a merged-not-deleted finding.

**AGM-WORKTREE-AUDIT-06** When a local branch is unmerged and older than the branch staleness threshold, the system shall report a stale-unmerged finding.

**AGM-WORKTREE-AUDIT-07** When the base ref cannot be resolved, the system shall omit merged/ahead-behind branch conclusions and warn that branch-level data is unreliable.

**AGM-WORKTREE-AUDIT-08** When JSON output is requested, the system shall emit root, generation time, thresholds, repository count, finding counts, and findings as one structured JSON report.

**AGM-WORKTREE-AUDIT-09** When text output is requested, the system shall group findings by stable finding kind and include repo, branch, last commit, age, ahead/behind, merged state, and detail columns.

**AGM-WORKTREE-AUDIT-10** When the command runs, the system shall never remove worktrees, delete branches, or mutate repositories.

## BDD Traceability

- Feature: `agm/test/bdd/features/workflow_tooling_guardrails.feature`

## Test Traceability

- Unit package: `cmd/agm-worktree-audit`
