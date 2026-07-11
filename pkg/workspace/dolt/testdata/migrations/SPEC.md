# Workspace Dolt Migration Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**WORKSPACE-DOLT-FIXTURE-01** When workspace migration tests load a SQL fixture, the system shall preserve the declared migration order and schema boundary.

**WORKSPACE-DOLT-FIXTURE-02** If a fixture represents an invalid migration, the system shall keep that failure deterministic for the migration test.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
