# TypeScript Code-Intelligence Rules Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-CODEINTEL-TS-01** When TypeScript source is analyzed, the system shall apply the versioned dead-function, debug-print, and unsafe-any rules.

**DECL-CODEINTEL-TS-02** If a TypeScript rule definition is invalid, the system shall fail rule loading rather than silently omit the check.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
