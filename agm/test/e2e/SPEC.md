# AGM End-to-End Tests Specification

<!-- Last audited at: 2026-07-23 -->

## Requirements

**E2E-01** When the end-to-end suite starts, the suite shall build or locate the AGM binary and isolate its filesystem and process state.

**E2E-02** When worker lifecycle behavior is exercised, the suite shall verify creation, observation, messaging, and cleanup through user-facing commands.

**E2E-03** When status-line behavior is exercised, the suite shall verify plain and JSON output, context thresholds, git state, and cache behavior.

**E2E-04** If an external harness prerequisite is unavailable, then the suite shall skip only the dependent scenario with an explicit reason.

**E2E-05** When harness-detection portability is enforced by unit and BDD surfaces, the suite shall route both through one canonical checker.

**E2E-06** When caching fallback AGM binaries in the user cache directory, the suite shall prune obsolete fixture directories exceeding the configured entry bound or age gate so that unbounded disk growth is prevented.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- End-to-end tests: `agm/test/e2e/*_test.go`
