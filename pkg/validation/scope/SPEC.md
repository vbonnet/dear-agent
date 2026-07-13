# Wayfinder Scope Validation Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**SCOPE-01** When markdown is parsed, the system shall extract ATX headings with formatting removed and one-indexed source lines.

**SCOPE-02** When headings are compared fuzzily, the system shall use Unicode-aware edit distance and the configured threshold.

**SCOPE-03** When a phase document is validated, the system shall check anti-patterns, required sections, and expected length unless explicitly skipped.

**SCOPE-04** When violations are classified, the system shall separate blocking errors from warnings and generate actionable recommendations.

**SCOPE-05** When validation override is enabled, the system shall retain findings while allowing the result to pass.

**SCOPE-06** When a validation report is formatted, the system shall preserve phase metadata, locations, severity, and suggestions.

**SCOPE-07** While phase documents are produced by any supported harness and model family, the system shall apply identical scope patterns and thresholds.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/validation/scope`
