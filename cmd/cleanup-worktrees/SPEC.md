# cleanup-worktrees Command Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 2.0
**Status:** Baseline
**Scope:** `cmd/cleanup-worktrees`.

## Overview

`cleanup-worktrees` reports the reclaimable git worktrees of one repository
and, with `--fix`, removes only those that positively prove reapable.

The safety model is an allowlist inherited from the canonical sweep in
`agm/internal/ops/worktree_sweep.go`: every worktree is assigned exactly one
classification, and only `MERGED` is ever removed. Every other verdict, every
failed probe, and every worktree the tool cannot reason about is kept. Age is
reported but never authorizes a deletion, and remote branches are never
touched.

This is a repository-scoped complement to `agm worktree sweep`, which
discovers worktrees by filesystem base directory instead. The two share the
same reapability stance deliberately: the 2026-05 sprawl retros and the
2026-07-10 hook incident both ended in live worktrees being deleted by a
cleanup that classified on a denylist.

## Classifications

| Class | Meaning | Removable |
| --- | --- | --- |
| `PROTECTED` | Primary checkout, the invoking worktree, a `git worktree lock`ed checkout, a `--preserve` name, a bare repository, or a worktree holding the target branch | No |
| `ACTIVE` | A live tmux/AGM session owns the checkout | No |
| `DIRTY` | Uncommitted or untracked changes are present | No |
| `MERGED` | Clean, unowned, and carrying no commits beyond the target ref | Yes |
| `ORPHANED` | Clean and unowned but carrying commits not on the target ref | No |
| `UNKNOWN` | A probe failed, or HEAD is detached, so safety is not established | No |

## EARS Requirements

**CLEANUP-WT-01** When no repository path is provided, the system shall print usage and reject the command.

**CLEANUP-WT-02** When more than one positional argument is provided, the system shall reject the command rather than guessing which path is the repository.

**CLEANUP-WT-03** When an unknown flag is provided, the system shall print usage and reject the command.

**CLEANUP-WT-04** When `-h` or `--help` is requested, the system shall print usage and exit without treating it as an error.

**CLEANUP-WT-05** When `--max-age` is provided without a value, or with a value that is not an integer, the system shall reject the command.

**CLEANUP-WT-06** When `--max-age` is not provided, the system shall default the reported idle threshold to fourteen days.

**CLEANUP-WT-07** When `--preserve` is provided without a value, the system shall reject the command.

**CLEANUP-WT-08** When a worktree directory name matches a `--preserve` value, the system shall classify it `PROTECTED` without evaluating any other rule.

**CLEANUP-WT-09** When the supplied repository path is not a git directory, the system shall fail with exit code 2 rather than operating on an unrelated directory.

**CLEANUP-WT-10** When the supplied repository path is relative, or names a subdirectory or a linked worktree, the system shall resolve it to its checkout root before comparing it against listed worktree paths.

**CLEANUP-WT-11** When resolving the target ref, the system shall fetch from `origin` first, then prefer the `origin/HEAD` symbolic ref, then fall back to `origin/main`, failing with exit code 2 when neither exists.

**CLEANUP-WT-12** When fetching from origin fails, the system shall warn and continue against cached refs rather than aborting the scan.

**CLEANUP-WT-13** When every git subprocess is launched, the system shall remove ambient repository selectors such as `GIT_DIR` and `GIT_WORK_TREE` from its environment, disable terminal prompting, and bound the call with a timeout.

**CLEANUP-WT-14** When enumerating worktrees, the system shall request NUL-delimited porcelain records so a path containing a newline is not truncated.

**CLEANUP-WT-15** When enumerating worktrees, the system shall classify the repository's primary checkout and the invoking process's own worktree as `PROTECTED`.

**CLEANUP-WT-16** When a worktree is bare, holds a `git worktree lock`, or has the target branch checked out, the system shall classify it `PROTECTED`.

**CLEANUP-WT-17** When a worktree has a detached HEAD, the system shall classify it `UNKNOWN` because no branch can be proven merged.

**CLEANUP-WT-18** When a live session name matches a worktree's directory name or its branch, the system shall classify that worktree `ACTIVE`.

**CLEANUP-WT-19** When the active-session probe cannot be completed, the system shall warn and remove no worktree during that run.

**CLEANUP-WT-20** When a worktree has uncommitted or untracked changes, the system shall classify it `DIRTY` and shall never remove it.

**CLEANUP-WT-21** When any git probe used for classification fails, the system shall classify that worktree `UNKNOWN` and keep it, so a failed probe never becomes a removal verdict.

**CLEANUP-WT-22** When a worktree carries no commits beyond the target ref and no preserving rule applies, the system shall classify it `MERGED`.

**CLEANUP-WT-23** When a worktree carries commits that are not on the target ref, the system shall classify it `ORPHANED` regardless of its age, and shall report the idle age without treating it as removable.

**CLEANUP-WT-24** When `--fix` is not provided, the system shall report the removal commands it would run, using the same short branch name the removal would use, and shall not mutate any worktree, branch, or remote.

**CLEANUP-WT-25** When `--fix` is provided and a worktree is classified `MERGED`, the system shall remove the checkout without `--force`, so git's own refusal to drop a dirty or locked checkout guards against a race with the classification probes.

**CLEANUP-WT-26** When worktree removal fails, the system shall preserve the associated local branch and count the removal as failed.

**CLEANUP-WT-27** When a worktree is removed, the system shall attempt a non-forced deletion of its local branch, and shall warn and keep the branch if git declines.

**CLEANUP-WT-28** When any cleanup runs, the system shall never delete a remote branch.

**CLEANUP-WT-29** When the scan completes, the system shall emit a summary reporting the count of each classification alongside removed and failed counts.

**CLEANUP-WT-30** When any removal failed, the system shall exit with code 3 so an automated caller can detect partial cleanup.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/cleanup-worktrees`

## Non-Goals

- The command does not consult GitHub pull request state. A squash-merged branch still carries commits that are not on the target ref, so it is classified `ORPHANED` and kept. `agm worktree sweep --check-pr` owns the PR oracle.
- The command does not discover worktrees across repositories. `agm worktree sweep` owns filesystem-based discovery beneath a worktrees base directory.
