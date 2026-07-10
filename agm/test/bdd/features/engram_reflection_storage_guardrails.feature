# SPEC: engram/internal/reflection/SPEC.md
# RELATED-SPEC: engram/internal/scanners/SPEC.md
# RELATED-SPEC: engram/internal/providers/simple/SPEC.md
Feature: Engram reflection and storage guardrails
  Engram reflection, project scanners, and the simple memory provider should
  carry executable SPEC traceability so learning and durable memory behave the
  same across supported harnesses and model providers.

  Scenario Outline: Engram reflection and storage packages declare SPEC coverage
    Given Engram reflection storage package "<package>" is configured
    When AGM validates Engram reflection storage package coverage
    Then Engram reflection storage package "<package>" should have a co-located SPEC

    Examples:
      | package                          |
      | engram/internal/reflection       |
      | engram/internal/scanners         |
      | engram/internal/providers/simple |
