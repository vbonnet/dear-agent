# Worktree Safety Policy Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/ops/wtpolicy` centralizes conservative worktree merge and
operator-input policy used by cleanup and supervision operations.

## Requirements

**WTP-01** When worktree dirtiness cannot be established, the system shall treat the worktree as dirty and unsafe to remove.

**WTP-02** When evaluating whether a branch is merged, the system shall require positive ancestry or pull-request evidence and preserve unknown evidence in the verdict.

**WTP-03** When a branch has unpushed commits or an open pull request, the system shall not classify its worktree as provably merged.

**WTP-04** When locating a worktree transcript, the system shall derive only supported session transcript paths and report absence explicitly.

**WTP-05** When transcript content indicates a pending user question, the system shall classify the worktree as awaiting input and return a diagnostic detail.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/ops/wtpolicy/*_test.go`
