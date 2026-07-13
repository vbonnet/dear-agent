# AGM Contract Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-CONTRACT-01** When AGM exchanges Engram data or evaluates service objectives, the system shall validate messages and thresholds against the versioned contracts.

**DECL-CONTRACT-02** If contract data violates its schema or service objective definition, the system shall reject the invalid contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
