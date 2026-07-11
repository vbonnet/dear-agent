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

  Scenario: Reflection storage documents filename-safe session IDs
    Given Engram reflection storage SPEC "engram/internal/reflection/SPEC.md" is loaded
    Then the SPEC should require reflection session IDs to use only ASCII letters, ASCII digits, hyphen, or underscore

  Scenario: Simple provider documents storage hardening rules
    Given Engram reflection storage SPEC "engram/internal/providers/simple/SPEC.md" is loaded
    Then the SPEC should reject unsafe artifact identifiers before constructing artifact paths
    And the SPEC should require temporary files to be flushed before atomic replacement
    And the SPEC should require concurrent filesystem operations to be serialized
