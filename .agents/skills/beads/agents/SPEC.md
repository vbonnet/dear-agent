# Beads Agent Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-BEADS-01** When a harness loads the Beads agent configuration, the system shall expose the declared OpenAI-compatible agent behavior and tool contract.

**DECL-BEADS-02** If the agent configuration is malformed, the system shall fail validation before dispatching a Beads task.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
