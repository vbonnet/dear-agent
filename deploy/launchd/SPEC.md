# Launchd Service Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-LAUNCHD-01** When a dear-agent background service is installed on macOS, the system shall use the declared launchd label, command, environment, and lifecycle policy.

**DECL-LAUNCHD-02** If a service definition references an unavailable executable, the system shall fail installation or startup visibly.

**DECL-LAUNCHD-03** When a launchd guard monitors a directory for deletion, the system shall install that guard's program outside the monitored directory so the guard survives the deletion it detects.

**DECL-LAUNCHD-04** When an operator installs recurring dangerous-override review on macOS, the system shall install a root-owned system LaunchDaemon and root-owned audit executable that an unattended same-user agent cannot replace or disable through its GUI launchd domain.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
