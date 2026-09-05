# SPEC: agm/internal/audit/SPEC.md
# RELATED-SPEC: internal/driftaudit/SPEC.md
# RELATED-SPEC: pkg/adrlint/SPEC.md
# RELATED-SPEC: pkg/audit/config/SPEC.md
# RELATED-SPEC: pkg/audit/verifiers/SPEC.md
# RELATED-SPEC: tools/adr-lint/SPEC.md
# RELATED-SPEC: pkg/retrolint/SPEC.md
Feature: Audit package guardrails
  Audit support packages should carry executable SPEC traceability because
  parity governance depends on session audits, drift evidence, audit config,
  and verifier dispatch staying documented and test-enforced.

  Scenario Outline: Audit packages declare SPEC coverage
    Given audit package "<package>" is configured
    When AGM validates audit package coverage
    Then audit package "<package>" should have a co-located SPEC

    Examples:
      | package             |
      | agm/internal/audit  |
      | internal/driftaudit |
      | pkg/adrlint         |
      | pkg/audit/config    |
      | pkg/audit/verifiers |
      | tools/adr-lint      |
      | pkg/retrolint       |

  Scenario Outline: Audit specifications define optional host dependencies
    Given audit package "<package>" is configured
    When AGM validates audit package coverage
    Then audit package SPEC should declare requirement "<requirement>" containing "<contract>"

    Examples:
      | package             | requirement    | contract                 |
      | internal/driftaudit | DRIFT-AUDIT-02 | home directory is provided |
      | agm/internal/audit  | AGM-AUDIT-08   | not installed            |
      | pkg/retrolint       | RLINT-12       | bound execution with a timeout |
