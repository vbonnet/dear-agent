# Git And Worktree Safety Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/git` provides bounded repository, pull-request, and worktree
inspection plus guarded manifest commits and cleanup operations.

## Requirements

**GIT-01** When committing a manifest, the system shall operate from the containing repository and avoid including unrelated staged or unstaged files.

**GIT-02** When repository or pull-request state cannot be established, the system shall return an explicit unknown result instead of claiming a branch is merged.

**GIT-03** When listing worktrees, the system shall preserve branch, detached-head, and path state from Git's porcelain output.

**GIT-04** When evaluating worktree cleanup, the system shall retain dirty, unpushed, unmerged, or otherwise unsafe worktrees.

**GIT-05** When removing a worktree or branch, the system shall use the requested force policy and propagate Git failures.

**GIT-06** When resolving a comparison base, the system shall prefer the canonical remote main branch and report no base when the repository is unknown.

**GIT-07** When a caller supplies a directory inside a Git worktree, the system shall resolve the containing worktree root before inventory, removal, or ownership comparison.

**GIT-08** When listing worktrees, the system shall parse and preserve worktree lock state and lock reason from Git porcelain output.

**GIT-09** When unlocking a worktree, the system shall execute Git worktree unlock and succeed idempotently if the worktree is not locked.

**GIT-10** When locking a worktree, the system shall execute Git worktree lock with any requested reason and propagate Git failures.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/git/*_test.go`
