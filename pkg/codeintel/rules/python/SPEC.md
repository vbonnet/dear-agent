# Python Code-Intelligence Rules Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-CODEINTEL-PY-01** When Python source is analyzed, the system shall apply the versioned dead-function, debug-print, and bare-exception rules.

**DECL-CODEINTEL-PY-02** If a Python rule definition is invalid, the system shall fail rule loading rather than silently omit the check.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
