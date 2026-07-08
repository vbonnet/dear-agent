# SPEC: agm/internal/audit/SPEC.md
# RELATED-SPEC: internal/driftaudit/SPEC.md
# RELATED-SPEC: pkg/audit/config/SPEC.md
# RELATED-SPEC: pkg/audit/verifiers/SPEC.md
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
      | pkg/audit/config    |
      | pkg/audit/verifiers |
