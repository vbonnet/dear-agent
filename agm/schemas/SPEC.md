# AGM Protocol Schema Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-SCHEMA-01** When AGM validates sessions, messages, tool calls, or corpus-callosum data, the system shall use the corresponding versioned JSON schema.

**DECL-SCHEMA-02** If protocol data violates a required schema field or type, the system shall reject the payload.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
