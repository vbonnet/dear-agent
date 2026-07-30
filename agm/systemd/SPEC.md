# AGM Systemd Service Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-SYSTEMD-01** When AGM is installed as a systemd service, the system shall use the declared daemon and boot-resume units.

**DECL-SYSTEMD-02** If a required executable or service dependency is unavailable, the system shall fail service startup visibly.

**DECL-SYSTEMD-03** When an operator stages recurring dangerous-override review on Linux, the system shall install a persistent daily user timer that invokes `agm override audit --notify`.

**DECL-SYSTEMD-04** When a scheduled Linux dangerous-override audit breaches its threshold, the system shall deliver the alert to the system journal under the `dear-agent-override-audit` identifier.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
