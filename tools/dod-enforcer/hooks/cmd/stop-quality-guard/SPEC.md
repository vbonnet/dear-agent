# Stop Quality Guard Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Harness-neutral stop-hook checks for unfinished code and missing documentation.

## EARS Requirements

**STOP-GUARD-01** When a stop event is evaluated, the system shall inspect changed source files for unfinished-code markers.

**STOP-GUARD-02** When `TODO`, `FIXME`, or `HACK` markers remain in changed source, the system shall report the marker as a quality failure.

**STOP-GUARD-03** When no unfinished-code markers remain, the system shall pass the code-marker check.

**STOP-GUARD-04** When project documentation is evaluated, the system shall require repository-level README documentation.

**STOP-GUARD-05** When the stop hook is invoked by any supported harness, the system shall apply the same quality checks without selecting a model provider.

## Test Traceability

- Package tests: `tools/dod-enforcer/hooks/cmd/stop-quality-guard/main_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
