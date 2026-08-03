# Safe Git Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/safegit`.

## Overview

`internal/safegit` implements the policy core for safe local git operations:
pushes that cannot hang on GUI credential helpers, rebases that avoid protected
branches and audit outcomes, and merges that require CI, review-thread, soak,
and optional reviewer/flake gates. These functions back the sanctioned wrappers
used by agents instead of raw git or raw GitHub merge commands.

## EARS Requirements

**SAFEGIT-01** When a push request contains a force flag, mirror flag, or force refspec, the system shall reject the push before invoking git.

**SAFEGIT-02** When building push arguments, the system shall clear inherited credential helpers and install the GitHub CLI helper as the only helper.

**SAFEGIT-03** When a push exceeds its timeout, the system shall return an error that states the push did not complete.

**SAFEGIT-04** When a rebase targets a protected branch, the system shall reject the rebase before invoking git.

**SAFEGIT-05** When safe rebase completes or fails, the system shall append an audit entry describing the outcome.

**SAFEGIT-06** When safe merge is invoked without a positive PR number or owner/repo, the system shall reject the request.

**SAFEGIT-07** When safe merge gates run, the system shall require all CI checks to pass.

**SAFEGIT-08** When unresolved review threads exist and review checks are not skipped, the system shall block the merge.

**SAFEGIT-09** When the PR head commit has not met the minimum soak time, the system shall block the merge.

**SAFEGIT-10** When break-glass merge is requested without an interactive TTY or adequate reason, the system shall refuse the merge.

**SAFEGIT-11** When repo safe-merge configuration is malformed, the system shall fail loudly before running merge gates.

**SAFEGIT-12** When configured flaky checks fail for the first allowed occurrence, the system shall request the sanctioned rerun before treating the check as a hard block.

**SAFEGIT-13** When post-merge cleanup attempts linked-worktree removal and local branch deletion, the system shall preserve the primary worktree path from NUL-delimited porcelain output, execute both Git operations from that stable worktree with caller cancellation, a bounded per-command deadline, and bounded pipe draining, attempt worktree removal before conservative branch deletion, and report cleanup failures as warnings without changing the completed provider merge result.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `internal/safegit`
