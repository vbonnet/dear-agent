# AGM Workflow Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-AGM-CI-01** When AGM package automation runs, the system shall execute its contract, installation, release, and test workflows for declared events.

**DECL-AGM-CI-02** If an AGM workflow gate fails, the system shall prevent that workflow from reporting a successful release or test result.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
