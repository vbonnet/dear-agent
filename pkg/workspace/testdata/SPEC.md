# Workspace Configuration Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-WORKSPACE-01** When workspace tests load fixtures, the system shall preserve valid, minimal, duplicate-name, invalid-version, empty, and environment-expanded cases.

**FIX-WORKSPACE-02** If invalid workspace configuration is accepted, the system shall fail the workspace scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
