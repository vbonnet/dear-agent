# AGM End-to-End Test Library Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-E2E-LIB-01** When an end-to-end suite requests a harness or Dolt fixture, the system shall resolve it through shared portable helpers.

**AGM-E2E-LIB-02** If a required fixture is unavailable, the system shall report a deterministic skip or failure rather than continue with partial state.

**AGM-E2E-LIB-03** When an end-to-end suite resolves harness commands, the system shall use syntax supported by macOS system Bash 3.2 while retaining the exact harness-to-executable mapping.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
