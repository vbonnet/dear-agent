# Agentic Review Verdict Action Specification

<!-- Last audited at: 2026-09-03 -->

## Overview

`.github/actions/agentic-review-verdict` resolves a finished reviewer family to
`posted` plus a verdict, or to the error state when it could not reach one. It
is the companion to `agentic-review-label`, which publishes a single phase; this
action decides which phase a finished reviewer earned.

## EARS Requirements

**ARV-01** When the reviewer step did not succeed, the action shall publish the error state so the gate can degrade around a down family without waiting out its deadline.

**ARV-02** When the reviewer succeeded, the action shall publish the posted label before publishing any verdict, so evidence that a review ran is recorded independently of whether it passed.

**ARV-03** When a review bound to the reviewed head by one of the configured reviewer logins requests changes, the action shall publish the changes-requested label.

**ARV-04** When such a review approves, the action shall publish the approved label.

**ARV-05** When the reviewer succeeded but left no review by a configured login on the reviewed head, the action shall fail rather than publish, so a misconfigured login surfaces immediately instead of decaying into a silently quorum'd-around down family.

## Test Traceability

- Feature: `agm/test/bdd/features/agentic_review_gate_guardrails.feature`
- Workflow wiring: `tests/bats/agentic-review-gate.bats`
- Decision consequence: `internal/agenticreview/*_test.go`
