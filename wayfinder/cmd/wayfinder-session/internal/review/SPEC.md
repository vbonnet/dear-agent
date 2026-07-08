# Wayfinder Review Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`wayfinder/cmd/wayfinder-session/internal/review` runs risk-adaptive,
multi-persona reviews for Wayfinder tasks. It calculates task risk from file
size, criticality, change type, coverage signals, and risky patterns; then it
chooses per-task or batch review strategy and aggregates persona findings into
blocking issues, scores, metrics, and recommendations.

## EARS Requirements

**WAYFINDER-REVIEW-01** When default review configuration is created, the system shall set assertion density, coverage, timeout, risk threshold, review trigger, and retry defaults.

**WAYFINDER-REVIEW-02** When a task risk is calculated, the system shall combine lines of code, file criticality, change type, coverage risk, and risky pattern signals into a composite risk level.

**WAYFINDER-REVIEW-03** When risk strings are parsed, the system shall accept XS, S, M, L, and XL case-insensitively and shall map unknown values to M.

**WAYFINDER-REVIEW-04** When per-task review trigger is evaluated, the system shall trigger only when calculated risk is at or above the configured per-task threshold.

**WAYFINDER-REVIEW-05** When batch review trigger is evaluated, the system shall ignore already reviewed tasks and shall reject the batch if any unreviewed task exceeds the configured batch maximum risk.

**WAYFINDER-REVIEW-06** When a task review runs, the system shall run spec-compliance review before code-quality personas.

**WAYFINDER-REVIEW-07** When code-quality personas are selected for a task, the system shall include security, performance, and maintainability for all risks, add UX for M or higher, and add reliability for L or higher.

**WAYFINDER-REVIEW-08** When batch review runs, the system shall review combined deliverables with spec-compliance first and a lighter security, performance, and maintainability persona set.

**WAYFINDER-REVIEW-09** When maintainability reviews Go files, the system shall attempt `golangci-lint run ./...` from the project directory before falling back to internal checks.

**WAYFINDER-REVIEW-10** When persona findings are aggregated, the system shall count severities, set per-persona scores, default missing persona scores to 100, and record review duration.

**WAYFINDER-REVIEW-11** When blocking issues are extracted, the system shall block on P0 and P1 issues and shall also block on P2 issues for XL-risk work.

**WAYFINDER-REVIEW-12** When a review report is generated, the system shall preserve task ID, risk level, review type, persona results, aggregate score, blocking issues, recommendations, pass status, and metrics.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_lifecycle_guardrails.feature`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/review/review_engine_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/review/harness_profile_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/review/parse_risk_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/review/persona_integration_test.go`
- Package tests: `wayfinder/cmd/wayfinder-session/internal/review/two_stage_test.go`

