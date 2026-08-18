# Branch Reaper Command Specification

<!-- Last audited at: 2026-08-17 -->

## Overview

`cmd/branch-reaper` is the retry-and-visibility layer for remote branch
cleanup. GitHub's "automatically delete head branches" setting auto-deletes
most merged PR branches but silently misses a small fraction, and it never
covers branches that never got a PR at all. This command classifies every
remote branch into one auto-actionable bucket and three human-review buckets,
and can delete the auto-actionable bucket on request.

## EARS Requirements

**BRR-01** When remote branches are enumerated, the command shall exclude the remote HEAD symref, the protected `main`/`master` branches, and `dependabot/*` branches.

**BRR-02** When a branch has an open pull request, the command shall take no action on it and shall not include it in any reported bucket.

**BRR-03** When a branch's most-recently-merged pull request's `mergedAt` timestamp is at or after the branch's tip commit's committer date, the command shall classify the branch as `safe_delete`.

**BRR-04** When a branch's most-recently-merged pull request's `mergedAt` timestamp is before the branch's tip commit's committer date, or either timestamp cannot be parsed, the command shall classify the branch as `review_new_commits_after_merge`.

**BRR-05** When a branch has a closed, non-merged pull request and no merged or open pull request, the command shall classify the branch as `review_closed_unmerged`.

**BRR-06** When a branch has no pull request at all, the command shall classify the branch as `review_no_pr`.

**BRR-07** When `--json` is given, the command shall emit a single JSON object with the `safe_delete`, `review_no_pr`, `review_closed_unmerged`, and `review_new_commits_after_merge` keys, each an array (never `null`) of branch names.

**BRR-08** When `--execute` is given, the command shall delete every branch in the `safe_delete` bucket from the `origin` remote after emitting the report, and shall report any individual deletion failure without aborting the run.

**BRR-09** When the target repository is not given via `GH_REPO`, the command shall infer it via `gh repo view`, and shall exit with status 3 and a descriptive message when neither source yields a repository.

**BRR-10** When the run completes, the command shall exit 0 if the three review buckets are all empty, exit 1 if any review bucket is non-empty, and exit 3 on a usage or environment error.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/branch-reaper/*_test.go`
