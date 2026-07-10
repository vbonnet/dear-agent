# Wayfinder Validator Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 2.0
**Status**: Active
**Scope**: Deterministic transition gates for the canonical nine-phase Wayfinder workflow.

## EARS Requirements

**WAYFINDER-VALIDATOR-01** When a phase start is requested, the system shall accept only a canonical phase and require the preceding non-skipped phase to be complete.

**WAYFINDER-VALIDATOR-02** When DESIGN is started, the system shall require a bounded `RESEARCH-existing-solutions.md` with overlap analysis, search methodology when reuse is incomplete, and at least 200 words.

**WAYFINDER-VALIDATOR-03** When a phase completion is requested, the system shall require the phase to be in progress and its canonical deliverable to contain meaningful content.

**WAYFINDER-VALIDATOR-04** When SPEC is completed, the system shall run the deterministic strict EARS gate without requiring a model provider.

**WAYFINDER-VALIDATOR-05** When DESIGN or PLAN documentation is completed, the system shall apply the configured document-quality gate to the canonical artifact.

**WAYFINDER-VALIDATOR-06** When PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, or SETUP contains modified source code, the system shall reject completion as a phase-boundary violation.

**WAYFINDER-VALIDATOR-07** When BUILD is completed, the system shall require implementation evidence and reject design-only placeholder language.

**WAYFINDER-VALIDATOR-08** When code verification runs, the system shall use fixed command arguments, bounded execution time, contained paths, file-size limits, and successful build and test results.

**WAYFINDER-VALIDATOR-09** When unresolved clarification markers, unchecked canonical assumption lists, or pending questions exist, the system shall reject phase completion with remediation guidance.

**WAYFINDER-VALIDATOR-10** When a rewind is requested, the system shall require a completed canonical target that precedes the current phase.

**WAYFINDER-VALIDATOR-11** When normal validation receives a retired phase identifier, the system shall reject it rather than route through a compatibility gate.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/validator/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
