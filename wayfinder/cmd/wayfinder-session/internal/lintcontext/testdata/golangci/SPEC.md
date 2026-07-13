# Wayfinder Golangci Context Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-GOLANGCI-01** When Wayfinder lint-context tests inspect golangci-lint configuration, the system shall preserve the declared linter shape.

**FIX-GOLANGCI-02** If Go lint context detection omits or misclassifies the fixture, the system shall fail the scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
