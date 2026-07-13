# GitHub Actions Local Event Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-ACT-01** When local GitHub Actions validation runs, the system shall load a deterministic push event payload.

**DECL-ACT-02** If the event fixture is invalid, the system shall fail local workflow validation rather than fabricate event fields.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
