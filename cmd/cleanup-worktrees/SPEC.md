# cleanup-worktrees Command Specification

<!-- Last audited at: 2026-08-17 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/cleanup-worktrees`.

## Overview

`cleanup-worktrees` finds and removes stale git worktrees for a repository. It
replaces the former `scripts/cleanup-worktrees.sh` so the staleness rules are
unit-testable Go rather than shell. A worktree is stale when it carries no
commits beyond the target ref, or when its last commit is older than the
configured maximum age. The command is dry-run by default; `--fix` is the
explicit opt-in that performs removals.

## EARS Requirements

**CLEANUP-WT-01** When no repository path is provided, the system shall print usage and reject the command.

**CLEANUP-WT-02** When more than one positional argument is provided, the system shall reject the command rather than guessing which path is the repository.

**CLEANUP-WT-03** When an unknown flag is provided, the system shall print usage and reject the command.

**CLEANUP-WT-04** When `-h` or `--help` is requested, the system shall print usage and exit without treating it as an error.

**CLEANUP-WT-05** When `--max-age` is provided without a value, or with a value that is not an integer, the system shall reject the command.

**CLEANUP-WT-06** When `--max-age` is not provided, the system shall default the staleness age threshold to fourteen days.

**CLEANUP-WT-07** When `--preserve` is provided without a value, the system shall reject the command.

**CLEANUP-WT-08** When a worktree directory name matches a `--preserve` value, the system shall keep that worktree and count it as preserved without evaluating staleness.

**CLEANUP-WT-09** When the supplied repository path is not a git directory, the system shall fail with exit code 2 rather than operating on an unrelated directory.

**CLEANUP-WT-10** When the target ref is resolved, the system shall prefer the `origin/HEAD` symbolic ref and shall fall back to `origin/main`, failing with exit code 2 when neither exists.

**CLEANUP-WT-11** When fetching from origin fails, the system shall warn and continue against cached refs rather than aborting the scan.

**CLEANUP-WT-12** When enumerating worktrees, the system shall skip the main repository checkout so the golden clone is never a removal candidate.

**CLEANUP-WT-13** When a worktree is bare or has a detached HEAD, the system shall leave it untouched.

**CLEANUP-WT-14** When a worktree has zero commits ahead of the target ref, the system shall classify it as stale with a merged-or-empty reason.

**CLEANUP-WT-15** When a worktree has commits ahead of the target ref but its last commit is at least `--max-age` days old, the system shall classify it as stale with an idle reason.

**CLEANUP-WT-16** When a worktree is neither preserved nor stale, the system shall keep it and count it as kept.

**CLEANUP-WT-17** When `--fix` is not provided, the system shall report the removal commands it would run and shall not mutate any worktree, branch, or remote.

**CLEANUP-WT-18** When `--fix` is provided and a stale worktree is found, the system shall remove the worktree, then delete its local branch, then delete the corresponding remote branch.

**CLEANUP-WT-19** When worktree removal fails, the system shall skip local and remote branch cleanup for that worktree and count the removal as failed, so a branch is never deleted while its checkout still exists.

**CLEANUP-WT-20** When local branch deletion fails, the system shall count the removal as failed and shall not attempt remote branch deletion.

**CLEANUP-WT-21** When remote branch deletion fails, the system shall log a note and shall not count the removal as failed, because an already-deleted remote branch is the expected post-merge state.

**CLEANUP-WT-22** When the scan completes, the system shall emit a summary reporting stale, kept, preserved, and failed counts.

**CLEANUP-WT-23** When any removal failed, the system shall exit with code 3 so an automated caller can detect partial cleanup.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/cleanup-worktrees`

## Non-Goals

- The command does not consult GitHub pull request state; merge status is inferred only from commits ahead of the target ref.
- The command does not inspect the working tree for uncommitted changes. Callers that need that protection must preserve the worktree explicitly with `--preserve`, or lock it.
