# Agentic Review Gate Command Specification

<!-- Last audited at: 2026-09-03 -->

## Overview

`cmd/agentic-review-gate` adapts GitHub to the per-family review policy in
`internal/agenticreview`. Its exit status is the verdict the `Agentic Review
Gate` job reports as the required check named by the `main` ruleset.

The command exists to keep the merge-blocking path cheap. The reviewers are the
expensive, quota-limited, occasionally-down part of the system, so nothing that
blocks a merge is allowed to depend on one of them answering a second time. This
command reads three GitHub endpoints and a policy file, and calls no model.

## EARS Requirements

**AGC-01** When evaluating a live pull request, the command shall read only the pull request view, its issue timeline, and its head commit, and shall invoke no model.

**AGC-02** When the verdict permits a merge, the command shall exit zero; when the reviewers decided against it, the command shall exit one; when the lifecycle has not resolved, the command shall exit three so a caller can distinguish a decision from an answer still coming.

**AGC-03** When the policy file is missing, unreadable, or invalid, the command shall exit with a usage error rather than evaluating against a built-in default.

**AGC-04** When neither an input file nor a repository and pull request are supplied, or when both are, the command shall exit with a usage error.

**AGC-05** When `--quorum` is supplied, the command shall evaluate against that threshold in place of the configured one, and shall report it in the verdict.

**AGC-06** When emitting a text summary, the command shall report the decision, every configured family's state and reason, the approval count, the down count, and the quorum.

**AGC-07** When a family published labels but is not configured, the command shall report it as a warning rather than counting it toward the quorum.

## Test Traceability

- Package tests: `cmd/agentic-review-gate/main_test.go`
- Scenario fixtures: `cmd/agentic-review-gate/testdata/*.json`
- Workflow wiring: `tests/bats/agentic-review-gate.bats`

## BDD Traceability

- Feature: `agm/test/bdd/features/agentic_review_gate_guardrails.feature`
- Steps: `agm/test/bdd/steps/agentic_review_gate_guardrails_steps.go`
