# Wayfinder Integration Workflow Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-WF-INTEGRATION-01** When Wayfinder integration automation runs, the system shall execute the declared integration workflow and preserve its test result.

**DECL-WF-INTEGRATION-02** If an integration job fails, the system shall not report the workflow as successful.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
