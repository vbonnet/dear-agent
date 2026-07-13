# GitHub Workflow Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-WORKFLOW-01** When repository automation is triggered, the system shall execute the versioned CI, security, audit, release, and maintenance workflows for the declared events.

**DECL-WORKFLOW-02** If a required workflow job fails, the system shall preserve the failing conclusion and shall not report the workflow as successful.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
