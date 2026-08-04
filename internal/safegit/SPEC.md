# Safe Git Specification

<!-- Last audited at: 2026-08-03 -->

**Version:** 1.1
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

**SAFEGIT-07** When safe merge gates run on a branch with required CI checks, the system shall require every provider-effective required check to pass and shall not block on failed informational checks.

**SAFEGIT-08** When unresolved review threads exist and review checks are not skipped, the system shall block the merge.

**SAFEGIT-09** When the PR head commit has not met the minimum soak time, the system shall block the merge.

**SAFEGIT-10** When break-glass merge is requested without an interactive TTY or adequate reason, the system shall refuse the merge.

**SAFEGIT-11** When repo safe-merge configuration is malformed, the system shall fail loudly before running merge gates.

**SAFEGIT-12** When configured flaky checks fail for the first allowed occurrence, the system shall request the sanctioned rerun before treating the check as a hard block.

**SAFEGIT-13** When the effective required-check policy cannot be completely discovered, the system shall block the merge before classifying CI results.

**SAFEGIT-14** When the effective base-branch policy is authoritatively known to require no CI checks, the system shall require every reported CI check to pass.

**SAFEGIT-15** When multiple branch policies apply, the system shall enforce the union of their required CI contexts and shall treat an unreported required context as pending.

**SAFEGIT-16** When a required CI context is scoped to a provider integration, the system shall use the provider's integration-aware required-check classification rather than context text alone, and shall block when multiple integration identities sharing one context cannot be proven independently.

**SAFEGIT-17** When an applicable required workflow cannot be proven complete, the system shall block the merge.

**SAFEGIT-18** When the provider's effective required-check projection contains a context absent from the discovered branch policy, the system shall block the merge as an incomplete-policy disagreement.

**SAFEGIT-19** When the provider returns valid check JSON with a documented failed-check or pending-check status exit, the system shall classify the returned checks rather than treating the status exit as a query failure.

**SAFEGIT-20** When classic branch protection repeats an app-scoped required context in both its legacy contexts and canonical checks fields, the system shall preserve only the canonical app-scoped identity.

**SAFEGIT-21** When another repository component needs required-check classification, the system shall expose and reuse one context-aware effective-policy projection with normalized passing, pending, and failing statuses.

**SAFEGIT-22** When the discovered policy is authoritatively empty and the provider reports that no required checks exist, the system shall accept the empty projection and shall preserve conservative validation of every reported check at the merge gate.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `internal/safegit`
