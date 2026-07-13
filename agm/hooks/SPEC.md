# AGM Runtime Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-RUNTIME-HOOK-01** When an AGM runtime hook receives lifecycle input, the system shall normalize variables before evaluating repository policy.

**AGM-RUNTIME-HOOK-02** If a roadmap or terminal policy check fails, the system shall stop the guarded action and preserve a diagnostic result.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
