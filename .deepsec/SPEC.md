# Deepsec Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DEEPSEC-01** When the repository security scanner loads its project configuration, the system shall select only declared source roots and matcher settings.

**DEEPSEC-02** If scanner configuration is invalid, the system shall fail before reporting a successful security scan.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
