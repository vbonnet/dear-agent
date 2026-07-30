# AGM Systemd Service Configuration Specification

<!-- Last audited at: 2026-07-29 -->

## EARS Requirements

**DECL-SYSTEMD-01** When AGM is installed as a systemd service, the system shall use the declared daemon and boot-resume units.

**DECL-SYSTEMD-02** If a required executable or service dependency is unavailable, the system shall fail service startup visibly.

**DECL-SYSTEMD-03** When an operator stages recurring dangerous-override review on Linux, the system shall install a persistent daily system timer that an unattended agent user cannot disable through its user manager.

**DECL-SYSTEMD-04** When the Linux system timer invokes a dangerous-override audit, the system shall run a root-owned audit executable as the named unprivileged operator and deliver threshold breaches to the system journal under the `dear-agent-override-audit` identifier.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
