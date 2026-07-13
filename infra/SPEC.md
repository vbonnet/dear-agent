# Repository Infrastructure Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**INFRA-01** When repository infrastructure is planned or applied, the system shall derive managed resources from versioned Terraform inputs.

**INFRA-02** If an import or apply operation cannot identify the intended resource, the system shall fail before mutating unrelated infrastructure state.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
