# AGM Shared Test Helpers Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**THELP-01** When a CLI helper runs AGM, the helper shall isolate home and XDG directories while preserving only the executable search path required by the test.

**THELP-02** When a tmux helper creates resources, the helper shall use a test-specific server and register deterministic cleanup.

**THELP-03** When a polling helper waits for asynchronous state, the helper shall honor context cancellation, timeout, and retry interval without fixed sleeps in callers.

**THELP-04** When contract tests consume API quota concurrently, the quota helper shall serialize consumption and never report a negative remainder.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/test/helpers/*_test.go`
