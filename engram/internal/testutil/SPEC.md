# Engram Test Utilities Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**ETU-01** When Engram tests create configuration, repositories, memories, or scanner manifests, the helpers shall write only beneath caller-provided temporary roots.

**ETU-02** When a helper creates test files, the helper shall use restrictive directory and file permissions where the platform supports them.

**ETU-03** If a test requires Unix permission semantics while running as root, then the helper shall skip that test with an explicit reason.

**ETU-04** When retrieval fixtures are created, the helpers shall return deterministic paths and valid maintained fixture content.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `engram/internal/testutil/*_test.go`
