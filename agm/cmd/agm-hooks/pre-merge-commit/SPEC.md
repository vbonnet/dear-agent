# Pre-Merge Commit CI Gate Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`pre-merge-commit` enforces local CI gate policy during git merge commits.

## EARS Requirements

**PMC-01** When the current commit is not a merge commit, the system shall skip CI gate execution.

**PMC-02** When a merge commit is detected, the system shall load the applicable CI gate policy for the merge target branch.

**PMC-03** When the policy allows a configured bypass and `SKIP_CI_GATES` requests it, the system shall allow the merge and record a bypass audit entry.

**PMC-04** When dry-run mode is requested, the system shall report the workflows that would run without executing them.

**PMC-05** When required workflows fail under a blocking policy, the system shall report remediation, roll back the merge, and return a failing exit code.

**PMC-06** When all required workflows satisfy the policy, the system shall allow the merge.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/pre-merge-commit/main_test.go`

