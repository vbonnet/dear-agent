# AGM Unit Tests Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**UNIT-01** When unit tests exercise AGM components, the suite shall isolate external processes and user filesystem state behind fakes or temporary resources.

**UNIT-02** When configuration, session, or tmux units are tested, the suite shall cover successful behavior and deterministic error handling.

**UNIT-03** When unit tests mutate environment or files, the suite shall restore those resources through the Go testing cleanup lifecycle.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Unit tests: `agm/test/unit/*_test.go`
