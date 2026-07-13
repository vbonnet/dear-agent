# Launchd Service Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-LAUNCHD-01** When a dear-agent background service is installed on macOS, the system shall use the declared launchd label, command, environment, and lifecycle policy.

**DECL-LAUNCHD-02** If a service definition references an unavailable executable, the system shall fail installation or startup visibly.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
