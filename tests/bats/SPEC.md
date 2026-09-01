# Repository Bats Test Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**BATS-01** When shell guardrail tests run, the system shall exercise approved and denied command paths through isolated fixtures.

**BATS-02** If a shell guard changes its user-facing remediation contract, the system shall require the corresponding Bats expectation to pass.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
