# Branch Reaper Command Specification

<!-- Last audited at: 2026-08-17 -->

## Overview

`cmd/branch-reaper` is the retry-and-visibility layer for remote branch
cleanup. GitHub's "automatically delete head branches" setting auto-deletes
most merged PR branches but silently misses a small fraction, and it never
covers branches that never got a PR at all. This command classifies every
remote branch into one auto-actionable bucket, three human-review buckets,
and one operational-failure bucket for branches it could not classify at
all, and can delete the auto-actionable bucket on request.

Redundancy is established by commit identity, not by timestamps: a branch is
only auto-deletable when its tip is the exact commit GitHub recorded as the
merged PR's head. A committer date cannot carry that proof, because a
force-push after the merge can put an older commit — carrying unmerged work —
on the branch tip.

The protected-branch set is derived, not hardcoded: it is the repository's
actual default branch plus every branch referenced by a `branches:` trigger
across `.github/workflows/*.yml`, with `main`/`master` kept as a fixed floor
in case dynamic detection fails outright. A branch whose PR history cannot be
retrieved (auth, rate limit, transient API error, or a PR count so large the
per-branch query is truncated) is reported in its own bucket instead of being
classified from incomplete data, and does not stop the rest of the run.

## EARS Requirements

**BRR-01** When remote branches are enumerated, the command shall exclude the remote HEAD symref, the protected branches (the repository's default branch, `main`, `master`, and any branch referenced by a `branches:` trigger in `.github/workflows/*.yml`), and `dependabot/*` branches.

**BRR-02** When a branch has an open pull request, the command shall take no action on it and shall not include it in any reported bucket.

**BRR-03** When a branch's most-recently-merged pull request records a head commit equal (case-insensitively) to the branch's tip commit SHA, the command shall classify the branch as `safe_delete`.

**BRR-04** When a branch's most-recently-merged pull request records a head commit that differs from the branch's tip commit SHA, or records no head commit at all, the command shall classify the branch as `review_new_commits_after_merge`.

**BRR-05** When a branch has a closed, non-merged pull request and no merged or open pull request, the command shall classify the branch as `review_closed_unmerged`.

**BRR-06** When a branch has no pull request at all, the command shall classify the branch as `review_no_pr`.

**BRR-07** When `--json` is given, the command shall emit a single JSON object with the `safe_delete`, `review_no_pr`, `review_closed_unmerged`, `review_new_commits_after_merge`, `lookup_failed`, `deleted`, and `delete_failed` keys, each an array (never `null`) of branch names.

**BRR-08** When `--execute` is given, the command shall delete every branch in the `safe_delete` bucket from the `origin` remote before emitting the report, leasing each deletion to the tip SHA the branch was classified against so that a ref updated since classification is rejected rather than deleted, and shall record each branch in `deleted` or `delete_failed` according to the outcome so that no branch still present on the remote is reported as deleted.

**BRR-09** When the target repository is not given via `GH_REPO`, the command shall infer it via `gh repo view`, and shall exit with status 3 and a descriptive message when neither source yields a repository.

**BRR-10** When a branch's pull-request history cannot be retrieved, or when that history is truncated by the per-branch fetch limit, the command shall classify the branch as `lookup_failed` instead of any other bucket, shall not delete it, and shall continue classifying the remaining branches rather than aborting the run.

**BRR-11** When a pull request originates from a fork, the command shall exclude it from the branch's history, because the head-branch-name filter matches fork branches that are unrelated to this repository's branch of the same name.

**BRR-12** When determining the protected-branch set, the command shall include the repository's default branch (via `gh repo view`) and every branch referenced by a `push` or `pull_request` `branches:` trigger in any `.github/workflows/*.yml` file, in addition to the fixed `main`/`master` floor, and a failure to determine the default branch or to read the workflows directory shall not be treated as an error.

**BRR-13** When the run completes, the command shall exit 0 if the four review/failure buckets (`review_no_pr`, `review_closed_unmerged`, `review_new_commits_after_merge`, `lookup_failed`) are all empty and no `--execute` deletion failed; shall exit 1 if any of the three review buckets is non-empty and `lookup_failed` is empty; shall exit 2 if `lookup_failed` is non-empty; shall exit 3 on a usage or environment error; and shall exit 4 when one or more safe deletions failed (which takes precedence over exit 1/2).

**BRR-14** When `--execute` is given and the `origin` remote does not resolve to the repository whose pull-request history was read, the command shall delete nothing and exit with status 3, because deletions target `origin` while classification targets that repository.

**BRR-15** When any `git` or `gh` subprocess is invoked, the command shall bound it with a deadline, so that a stalled call is surfaced as that one branch's failure rather than starving the rest of the run.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/branch-reaper/*_test.go`
