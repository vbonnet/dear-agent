# Configuration Loader Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-CONFIG-LOADER-01** When configuration-loader tests read fixtures, the system shall preserve empty, invalid, nested, valid, and home-expanded cases.

**FIX-CONFIG-LOADER-02** If invalid configuration is accepted or home expansion drifts, the system shall fail the loader scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
