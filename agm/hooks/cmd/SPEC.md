# AGM Hook Command Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-HOOK-CMD-01** When an AGM hook command observes a session event, the system shall publish the corresponding state transition through the configured channel.

**AGM-HOOK-CMD-02** If event context is incomplete, the system shall avoid publishing a misleading ready or completion state.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
