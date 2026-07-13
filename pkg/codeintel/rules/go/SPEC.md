# Go Code-Intelligence Rules Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-CODEINTEL-GO-01** When Go source is analyzed, the system shall apply the versioned dead-function and debug-print rules.

**DECL-CODEINTEL-GO-02** If a Go rule definition is invalid, the system shall fail rule loading rather than silently omit the check.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
