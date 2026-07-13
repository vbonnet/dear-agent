# Engram Health Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-HEALTH-01** When Engram health tests load marketplace and settings fixtures, the system shall distinguish valid configuration from invalid sources, paths, and extensions.

**FIX-HEALTH-02** If broken health configuration is reported as valid, the system shall fail the health scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
