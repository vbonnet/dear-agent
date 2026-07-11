# AGM Bus Channel Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-BUS-CHANNEL-01** When the plugin exchanges an AGM bus message, the system shall encode and decode the canonical wire shape without losing routing metadata.

**AGM-BUS-CHANNEL-02** If the broker connection closes or returns malformed data, the system shall surface the failure without treating the message as delivered.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
