# Bumblebee Service Template Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-BUMBLEBEE-01** When Bumblebee deployment renders its service template, the system shall substitute canonical executable and environment paths.

**DECL-BUMBLEBEE-02** If required template data is absent, the system shall fail before installing an incomplete service.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
