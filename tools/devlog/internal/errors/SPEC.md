# Devlog Error Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Structured error context and exit-code classification for devlog.

## EARS Requirements

**DEVLOG-ERROR-01** When an operation error is wrapped, the system shall preserve the original error for `errors.Is` and `errors.As` traversal.

**DEVLOG-ERROR-02** When a path-specific operation fails, the system shall include both operation and path context in the rendered error.

**DEVLOG-ERROR-03** When a structured devlog error is rendered, the system shall omit empty context fields.

**DEVLOG-ERROR-04** When an error category is mapped to process status, the system shall return its stable documented exit code.

## Test Traceability

- Package tests: `tools/devlog/internal/errors/errors_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
