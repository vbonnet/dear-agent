# VROOM Decision Trail Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Append-only persistence for consequential VROOM decisions.

## EARS Requirements

**VROOM-TRAIL-01** When a decision trail is opened with an empty path, the system shall reject the request.

**VROOM-TRAIL-02** When a decision trail path has missing parent directories, the system shall create the directories before opening the append-only file.

**VROOM-TRAIL-03** When a record has no event identifier, the system shall assign a unique event identifier before writing it.

**VROOM-TRAIL-04** When a record has no timestamp, the system shall assign a UTC timestamp before writing it.

**VROOM-TRAIL-05** When a record has a non-UTC timestamp, the system shall normalize the timestamp to UTC before writing it.

**VROOM-TRAIL-06** When a record is appended, the system shall encode it as one complete JSON line without rewriting existing records.

**VROOM-TRAIL-07** While concurrent callers append records, the system shall serialize complete record writes so that JSON lines do not interleave.

**VROOM-TRAIL-08** When an append context is canceled, the system shall return the context error without writing the record.

**VROOM-TRAIL-09** When a trail is closed more than once, the system shall treat subsequent close operations as successful no-ops.

**VROOM-TRAIL-10** When an append is attempted after close, the system shall reject the append.

## Test Traceability

- Package tests: `pkg/vroom/decisiontrail/trail_test.go`
- BDD: `agm/test/bdd/features/vroom_runtime_guardrails.feature`
