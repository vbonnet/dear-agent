# SPEC: engram/cmd/engram/internal/cli/SPEC.md
# RELATED-SPEC: engram/cmd/engram/cmd/SPEC.md
# RELATED-SPEC: engram/cmd/engram/internal/validation/SPEC.md
# RELATED-SPEC: engram/internal/slashcmd/SPEC.md
# RELATED-SPEC: engram/internal/tableutil/SPEC.md
Feature: Engram CLI support guardrails
  Engram CLI helpers, validation, slash commands, and table formatting should
  carry executable SPEC traceability so operator surfaces stay safe and
  predictable across harnesses.

  Scenario Outline: Engram CLI support packages declare SPEC coverage
    Given Engram CLI support package "<package>" is configured
    When AGM validates Engram CLI support package coverage
    Then Engram CLI support package "<package>" should have a co-located SPEC

    Examples:
      | package                               |
      | engram/cmd/engram/cmd                 |
      | engram/cmd/engram/internal/cli        |
      | engram/cmd/engram/internal/validation |
      | engram/internal/slashcmd              |
      | engram/internal/tableutil             |
