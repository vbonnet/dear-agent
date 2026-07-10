# SPEC: engram/internal/enforcement/SPEC.md
# RELATED-SPEC: engram/internal/guidance/SPEC.md
# RELATED-SPEC: engram/internal/harnesseffort/SPEC.md
# RELATED-SPEC: engram/internal/platform/SPEC.md
Feature: Engram governance runtime guardrails
  Engram enforcement, guidance, harness-effort, and platform packages should
  carry executable SPEC traceability so governance remains harness-neutral and
  custom model providers remain usable as parity expands.

  Scenario Outline: Engram governance runtime packages declare SPEC coverage
    Given Engram governance runtime package "<package>" is configured
    When AGM validates Engram governance runtime package coverage
    Then Engram governance runtime package "<package>" should have a co-located SPEC

    Examples:
      | package                       |
      | engram/internal/enforcement   |
      | engram/internal/guidance      |
      | engram/internal/harnesseffort |
      | engram/internal/platform      |
