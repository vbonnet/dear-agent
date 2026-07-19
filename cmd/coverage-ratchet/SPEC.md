# Coverage Ratchet Command Specification

<!-- Last audited at: 2026-07-19 -->

## Requirements

**COVERAGE-RATCHET-01** When a coverage policy is evaluated, the command shall run every declared package and fail if any package is below its minimum statement coverage.

**COVERAGE-RATCHET-02** If a policy is malformed, empty, duplicated, uses an unsupported version, or declares a percentage outside zero through one hundred, then the command shall fail without running package tests.

**COVERAGE-RATCHET-03** When a package test fails or exceeds its bounded timeout, the command shall preserve that failure and shall not report the coverage policy as passing.

## BDD Traceability

- Feature: `agm/test/bdd/features/quality_command_guardrails.feature`
- Package tests: `cmd/coverage-ratchet/main_test.go`
