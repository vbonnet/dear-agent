# Devlog Output Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Stable human-readable output through an injectable writer contract.

## EARS Requirements

**DEVLOG-OUTPUT-01** When callers emit success, information, or error messages, the system shall write the corresponding formatted message to the configured stream.

**DEVLOG-OUTPUT-02** When progress is emitted in verbose mode, the system shall write the progress message.

**DEVLOG-OUTPUT-03** When progress is emitted outside verbose mode, the system shall suppress the progress message.

**DEVLOG-OUTPUT-04** When tabular output has rows, the system shall render headers and aligned row values.

**DEVLOG-OUTPUT-05** When tabular output has no rows, the system shall complete without malformed output.

**DEVLOG-OUTPUT-06** When output behavior is consumed by commands, the system shall expose it through the provider-neutral `Writer` interface.

## Test Traceability

- Package tests: `tools/devlog/internal/output/output_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
