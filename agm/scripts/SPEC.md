# AGM Operational Scripts Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**ASCR-01** When the Go test-data generator is compiled, the scripts package shall remain buildable within the repository module.

**ASCR-02** When multi-workspace test data is generated, the generator shall use separate configured adapters and workspace-specific session identifiers.

**ASCR-03** If an optional secondary workspace is unavailable, then the generator shall report the condition and continue only with the available primary workspace.

**ASCR-04** When cleanup is requested, the generator shall clean test records before creating replacement fixtures.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/scripts/*_test.go`
