# safe-rebase Command Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `cmd/safe-rebase`.

## Overview

`safe-rebase` is the sanctioned CLI for rebasing feature worktrees. It parses
repository, base branch, dry-run, and automation options before delegating to
`internal/safegit` so protected branches, preflight, conflicts, and force-push
handling stay centralized.

## EARS Requirements

**SAFE-REBASE-01** When help is requested, the system shall print usage without treating it as an error.

**SAFE-REBASE-02** When rebase arguments are malformed, the system shall reject the command before invoking git.

**SAFE-REBASE-03** When a repository directory is not provided, the system shall default to the current directory.

**SAFE-REBASE-04** When a base branch is not provided, the system shall use the configured default base.

**SAFE-REBASE-05** When arguments are valid, the system shall delegate to the safe rebase policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- Unit package: `cmd/safe-rebase`
