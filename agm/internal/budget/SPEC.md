# AGM Context Budget Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/budget` computes per-session context budget status from manifests.
It applies role-specific warning thresholds, critical thresholds, and role
inference so supervisors can surface context pressure before sessions stall.

## EARS Requirements

**BUD-01** When a budget config is requested for a known role, the system shall use that role's default warning threshold.

**BUD-02** When a budget config is requested for an unknown role, the system shall fall back to the default role threshold.

**BUD-03** When a manifest has context usage, the system shall preserve used tokens, total tokens, and percentage used in the computed status.

**BUD-04** When usage is at or above the critical threshold, the system shall classify the status as critical.

**BUD-05** When usage is below critical but at or above the warning threshold, the system shall classify the status as warning.

**BUD-06** When role inference sees an explicit role tag, the system shall prefer that tag over parent or monitor heuristics.

**BUD-07** When a manifest has a parent session and no explicit role tag, the system shall infer worker role.

**BUD-08** When a manifest has monitors and no explicit role tag or parent, the system shall infer orchestrator role.

## BDD Traceability

- Feature: `agm/test/bdd/features/quota_parity.feature`
- Package tests: `agm/internal/budget/budget_test.go`
