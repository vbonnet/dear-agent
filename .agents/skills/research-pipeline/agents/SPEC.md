# Research Pipeline Agent Configuration Specification

<!-- Last audited at: 2026-07-26 -->

## EARS Requirements

**DECL-RESEARCH-PIPELINE-01** When a harness loads the research-pipeline agent configuration, the system shall expose the declared OpenAI-compatible agent behavior and tool contract.

**DECL-RESEARCH-PIPELINE-02** If the agent configuration is malformed, the system shall fail validation before dispatching a research-pipeline task.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
