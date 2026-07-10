# Deployment Drift Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/drift`.

## Overview

`internal/drift` compares repository sources of truth with host-deployed
artifacts. It provides a harness-neutral report for hooks, launch agents,
settings, and other files that must be deployed after source changes.

## EARS Requirements

**DRIFT-01** When drift configuration is parsed, the system shall reject empty configuration and unknown YAML fields.

**DRIFT-02** When a drift check starts without a repository root, the system shall refuse to guess a source location.

**DRIFT-03** When source and deployed content hashes match after token expansion, the system shall report the target as current.

**DRIFT-04** When source and deployed content hashes differ, the system shall report drift with hashes, a human-readable difference, and remediation guidance.

**DRIFT-05** When a required deployed artifact is missing, the system shall count actionable drift and retain remediation guidance.

**DRIFT-06** When an optional deployed artifact is missing, the system shall report it as skipped rather than actionable drift.

**DRIFT-07** When a git reference is configured, the system shall read source content from that reference instead of uncommitted working-tree content.

**DRIFT-08** When a target contains home or environment tokens, the system shall expand them before comparison.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/drift`
