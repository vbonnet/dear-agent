# pr-blockers Command Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/pr-blockers` and the classifier core in `internal/safegit/blockers.go`.

## Overview

`pr-blockers` is the deterministic PR merge-blocker classifier. Given a pull
request number it reports, from GitHub's authoritative merge state, the exact
blocker set and the exact remediation for each, so agents never diagnose a
stuck merge by guesswork. It reads `gh pr view --json
mergeStateStatus,mergeable,isDraft,reviewDecision,...`, the paginated
reviewThreads GraphQL including `isOutdated`, and the provider-effective
required-check projection shared with safe-merge.

## EARS Requirements

**PR-BLOCKERS-01** When no PR number, a non-positive PR number, or more than one positional number is provided, the system shall print usage and exit with code 2.

**PR-BLOCKERS-02** When no repository is provided via flag or `GITHUB_REPOSITORY`, the system shall attempt to resolve the current directory's GitHub repository and shall exit with code 2 if none resolves.

**PR-BLOCKERS-03** When the pull request is merged or closed, the system shall report that verdict without classifying blockers.

**PR-BLOCKERS-04** When classifying an open pull request, the system shall page through every review thread and shall count unresolved threads as blockers regardless of their outdated status, labeling outdated ones.

**PR-BLOCKERS-05** When classifying an open pull request, the system shall report failing and pending provider-effective required checks by name as distinct blockers.

**PR-BLOCKERS-06** When the merge state is BEHIND, the system shall emit a blocker whose remediation is `gh pr update-branch`, ordered after all content blockers.

**PR-BLOCKERS-07** When the merge state is BLOCKED and no blocker is detected, the system shall emit an explicit unknown-block result that instructs re-query and branch-protection comparison and forbids speculative code investigation.

**PR-BLOCKERS-08** When no blockers are detected, the system shall report the READY verdict naming `safe-merge` as the merge path and exit with code 0.

**PR-BLOCKERS-09** When one or more blockers are detected, the system shall list them in remediation order with an exact fix each and exit with code 1.

**PR-BLOCKERS-10** When `--json` is provided, the system shall emit the full diagnosis as JSON on stdout.

## Test Traceability

- `cmd/pr-blockers/main_test.go`: usage errors, READY output, BLOCKED output contract.
- `internal/safegit/blockers_test.go`: per-blocker classification, ordering, outdated-thread blocking, unknown-block fallback.
- `internal/safegit/merge_test.go`: `TestParseReviewThreads_UnresolvedOutdatedBlocks` pins the outdated-thread gate fix.
