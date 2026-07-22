# AGM Cleanup Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/cleanup`.

## Overview

`agm/internal/cleanup` owns resource cleanup for AGM sessions and stale
worktrees. It removes tracked session worktrees, prunes eligible branches,
cleans temporary session files, and reaps stale worktrees only when conservative
git and PR-state checks prove they are safe to remove. Cleanup is deliberately
best-effort: errors are recorded for operators without allowing one failure to
hide the rest of the cleanup result.

## EARS Requirements

**CLEANUP-01** When session cleanup is called without a worktree store, the system shall return a cleanup result without attempting store operations.

**CLEANUP-02** When listing worktrees for a session fails, the system shall record the error in the cleanup result.

**CLEANUP-03** When a tracked worktree exists, the system shall request git worktree removal and untrack the worktree record.

**CLEANUP-04** When a tracked worktree is already absent, the system shall still untrack the worktree record.

**CLEANUP-05** When multiple worktrees share a repository branch cleanup target, the system shall deduplicate branch deletion attempts.

**CLEANUP-06** When worktree removal or branch deletion fails, the system shall record the error and continue cleanup.

**CLEANUP-07** When temporary files are associated with a session, the system shall remove those temporary files best-effort.

**CLEANUP-08** When stale worktree reaping is disabled, the system shall return without removing worktrees.

**CLEANUP-09** When a worktree is outside the configured worktrees base, the system shall keep that worktree.

**CLEANUP-10** When a worktree is dirty, the system shall keep that worktree.

**CLEANUP-11** When a worktree has commits ahead of base and PR state is unknown or unmerged, the system shall keep that worktree.

**CLEANUP-12** When a worktree has commits ahead of base and its PR is merged, the system shall allow removal if the worktree is otherwise safe.

**CLEANUP-13** When dry run is enabled, the system shall report removable worktrees without removing them.

**CLEANUP-14** When a tracked worktree was removed or was already absent, the system shall delete only that record's non-empty branch, deduplicate identical repository-and-branch targets, and preserve every inferred session-name branch and every branch whose worktree removal failed.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/internal/cleanup`
