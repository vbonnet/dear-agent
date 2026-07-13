# AGM Internal Test Utilities Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**AITU-01** When a test requires Engram isolation, the helper shall reject execution unless test mode and a test workspace are configured.

**AITU-02** When the test environment is initialized, the helper shall configure test mode and workspace variables while preserving caller-provided values.

**AITU-03** If a required Dolt test adapter is unavailable, then the helper shall skip the dependent test instead of using a production database.

**AITU-04** When a Dolt test adapter is available, the helper shall apply migrations before returning it to the test.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/internal/testutil/*_test.go`
