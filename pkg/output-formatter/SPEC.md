# Output Formatter Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**OUTPUT-01** When result status is rendered, the system shall use stable pass, warning, failure, and informational labels and icons.

**OUTPUT-02** When JSON output is requested, the system shall emit a valid machine-readable result array with optional indentation.

**OUTPUT-03** When JSON output includes a summary, the system shall preserve aggregate totals and categorized result details.

**OUTPUT-04** When a human summary is generated, the system shall count status levels and group issues by category.

**OUTPUT-05** When compact summary output is requested, the system shall preserve the same counts without verbose issue detail.

**OUTPUT-06** When no issues are present, the system shall produce a successful empty-issue representation.

**OUTPUT-07** While output is consumed by any supported harness and model family, the system shall preserve identical structured semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/output-formatter`
