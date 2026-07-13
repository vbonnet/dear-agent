# Shared Test Utilities Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**STU-01** When shared tests create configuration, repositories, memories, or scanner manifests, the helpers shall write only beneath caller-provided temporary roots.

**STU-02** When a helper modifies process environment, the helper shall use testing cleanup semantics so the caller's environment is restored.

**STU-03** If a test requires Unix permission semantics while running as root, then the helper shall skip that test with an explicit reason.

**STU-04** When shared fixtures are generated, the helpers shall produce deterministic valid content for the consuming package.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `internal/testutil/*_test.go`
