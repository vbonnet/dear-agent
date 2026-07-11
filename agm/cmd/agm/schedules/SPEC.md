# AGM Schedule Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-SCHEDULE-01** When AGM installs scheduled maintenance, the system shall use the declared launchd labels, commands, and intervals.

**DECL-SCHEDULE-02** If a scheduled command or path is invalid, the system shall fail installation before loading the schedule.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
