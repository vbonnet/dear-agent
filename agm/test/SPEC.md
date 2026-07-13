# AGM Test Root Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**ATST-01** When repository tests execute the AGM test root, the suite shall retain its benchmark build and execution sentinel.

**ATST-02** When test-only packages are audited for coverage, the repository shall treat them as governed package boundaries with co-located specifications.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/test/*_test.go`
