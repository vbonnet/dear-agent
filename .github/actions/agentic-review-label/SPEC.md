# Agentic Review Label Action Specification

<!-- Last audited at: 2026-09-03 -->

## Overview

`.github/actions/agentic-review-label` publishes one
`agentic-review:<family>:<phase>` label against the exact head a reviewer family
is reviewing. Every reviewer family calls it rather than writing labels itself,
so the rules that make the labels trustworthy have one implementation.

## EARS Requirements

**ARL-01** When the live pull request head differs from the head the reviewer read, the action shall skip the label write and report why, rather than recording a verdict against code the reviewer never saw.

**ARL-02** When the label does not yet exist in the repository, the action shall provision it, so adding a family or a phase needs no separate setup step.

**ARL-03** When provisioning a label that already exists, the action shall succeed rather than fail, so repeated reviews are idempotent.

**ARL-04** When the live head matches, the action shall apply the label to the pull request and report the head it was bound to.

## Test Traceability

- Feature: `agm/test/bdd/features/agentic_review_gate_guardrails.feature`
- Workflow wiring: `tests/bats/agentic-review-gate.bats`
- Decision consequence: `internal/agenticreview/*_test.go`
