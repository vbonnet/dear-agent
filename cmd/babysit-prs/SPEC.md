# Babysit PRs Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/babysit-prs` serializes safe rebases and guarded merges for repositories
that require linear history.

## EARS Requirements

**BPR-01** When open pull requests are listed, the command shall exclude drafts and process at most the configured limit.

**BPR-02** When the open pull-request count exceeds the configured cap, the command shall apply backpressure without attempting a merge.

**BPR-03** When a pull request is behind its base, the command shall request an update-branch rebase before merge evaluation.

**BPR-04** When a pull request is considered for merge, the command shall delegate the merge predicate and squash operation to `safe-merge`.

**BPR-05** When one pull request cannot be updated or merged, the command shall report that result and continue with later pull requests.

**BPR-06** When a per-pull-request timeout expires, the command shall terminate the child operation and return a contextual timeout error.

**BPR-07** When bot review is skipped, the command shall require and forward a non-empty audited reason.

**BPR-08** When dry-run mode is enabled, the command shall forward dry-run behavior to `safe-merge` and shall not perform a merge.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/babysit-prs/*_test.go`
