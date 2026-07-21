# Wayfinder status requirements specification

<!-- Last audited at: 2026-07-17 -->

**Status:** Active
**Scope:** `WAYFINDER-STATUS.md` schema, parsing, and persistence.

## EARS requirements

**WFSTATUS-01** When status is parsed, the system shall require the complete canonical V2 schema, including `schema_version: "2.0"`, required fields, valid enums, ordered history with completed mandatory predecessors, and valid conditional state.

**WFSTATUS-02** When status contains an unsupported schema version, the system shall return an error without normalization or fallback.

**WFSTATUS-03** When a phase is validated, the system shall accept only CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, or RETRO.

**WFSTATUS-04** When status is serialized, the system shall preserve project metadata, lifecycle diagnostics, phase history, roadmap tasks, and quality evidence.

**WFSTATUS-05** When status is written, the system shall use a temporary file and atomic replacement.

**WFSTATUS-06** When a phase transition succeeds, the system shall update current phase, timestamps, history, status, and outcome consistently.

**WFSTATUS-07** When skip settings are present, the system shall skip only the configured named phases and optional SETUP phase.

**WFSTATUS-08** When status is invalid, the system shall return field-specific validation errors.

**WFSTATUS-09** When lifecycle state is input-required, dependency-blocked, or failed, the system shall require input-needed, blocked-on, or error-message diagnostics respectively in addition to a blocked reason.

**WFSTATUS-10** When canonical YAML contains a field not declared by schema 2.0, the system shall reject it rather than silently discard it.

## Traceability

- Parser tests: `parser_v2_test.go`
- Schema validation tests: `validator_v2_test.go`
- Command integration: `../../commands/*_test.go`
- Status BDD: `agm/test/bdd/features/wayfinder_status_guardrails.feature`
- Command BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
