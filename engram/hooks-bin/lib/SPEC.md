# Engram Hook Validation Library Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**ENGRAM-HOOK-LIB-01** When a hook requests build or test validation, the system shall execute the declared validator and preserve its exit status.

**ENGRAM-HOOK-LIB-02** If a validator command is absent, the system shall report the missing prerequisite instead of treating validation as passed.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
