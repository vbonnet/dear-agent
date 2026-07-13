# Engram Hook Command Integration Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**EHCI-01** When command-level hook scenarios simulate passive waiting, batch work, context loss, missed verification, or task-tracking gaps, the suite shall verify the expected intervention.

**EHCI-02** When a session resumes, the command-level hook suite shall verify relevant context is displayed without leaking unrelated session state.

**EHCI-03** When hook scenarios create temporary state, the suite shall isolate it from the user's Engram configuration and memory stores.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Integration tests: `engram/hooks-bin/cmd/integration_test/*_test.go`
