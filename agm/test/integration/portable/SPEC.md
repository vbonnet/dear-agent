# AGM Portable Integration Specification

<!-- Last audited at: 2026-07-21 -->

## Requirements

**IPORT-01** When the integration graph runs with legacy end-to-end scenarios disabled, the suite shall still execute credential-free conformance for every active harness.

**IPORT-02** If one active harness is unavailable on the host, then the suite shall skip only that harness's availability probe and shall retain the shared conformance assertions.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Portable integration tests: `agm/test/integration/portable/*_test.go`
