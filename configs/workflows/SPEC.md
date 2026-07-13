# Workflow Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-WF-CONFIG-01** When a configured workflow is selected, the system shall load its declared stages, checks, signals, and constitutional constraints.

**DECL-WF-CONFIG-02** If workflow configuration is invalid, the system shall fail before starting workflow execution.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
