# AGM Systemd Service Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-SYSTEMD-01** When AGM is installed as a systemd service, the system shall use the declared daemon and boot-resume units.

**DECL-SYSTEMD-02** If a required executable or service dependency is unavailable, the system shall fail service startup visibly.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
