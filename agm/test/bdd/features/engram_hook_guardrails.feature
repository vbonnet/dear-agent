# SPEC: engram/hooks/SPEC.md
# RELATED-SPEC: engram/hooks/builtin/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/validator/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/worktree/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/verification/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/analyzer/SPEC.md
Feature: Engram hook guardrails
  Engram hook runtime and hook-bin enforcement packages should carry executable
  SPEC traceability so hook parity cannot regress into undocumented
  Claude-only behavior.

  Scenario Outline: Engram hook packages declare SPEC coverage
    Given Engram hook package "<package>" is configured
    When AGM validates Engram hook package coverage
    Then Engram hook package "<package>" should have a co-located SPEC

    Examples:
      | package                                |
      | engram/hooks                           |
      | engram/hooks/builtin                   |
      | engram/hooks-bin/internal/validator    |
      | engram/hooks-bin/internal/worktree     |
      | engram/hooks-bin/internal/verification |
      | engram/hooks-bin/internal/analyzer     |

