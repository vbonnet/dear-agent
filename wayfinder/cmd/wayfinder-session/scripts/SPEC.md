# Wayfinder Session Script Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**WF-SESSION-SCRIPT-01** When schema registration scripts run, the system shall register or unregister the requested Wayfinder schema explicitly.

**WF-SESSION-SCRIPT-02** When integration scripts exercise Claude Code, the system shall preserve failures instead of treating unavailable integration behavior as verified.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
