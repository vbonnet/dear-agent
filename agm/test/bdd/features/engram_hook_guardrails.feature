# SPEC: engram/hooks/SPEC.md
# RELATED-SPEC: engram/hooks/builtin/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/validator/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/worktree/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/verification/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/analyzer/SPEC.md
# RELATED-SPEC: engram/hooks-bin/cmd/hook-analyzer/SPEC.md
# RELATED-SPEC: engram/hooks-bin/cmd/prepush-act-validator/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/beads/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/context/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/git/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/goldenref/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/heartbeat/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/limiter/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/pivot/SPEC.md
# RELATED-SPEC: engram/hooks-bin/internal/session/SPEC.md
# RELATED-SPEC: engram/hooks/cmd/stop-retrospect/SPEC.md
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
      | engram/hooks-bin/cmd/hook-analyzer     |
      | engram/hooks-bin/cmd/prepush-act-validator |
      | engram/hooks-bin/internal/beads        |
      | engram/hooks-bin/internal/context      |
      | engram/hooks-bin/internal/git          |
      | engram/hooks-bin/internal/goldenref    |
      | engram/hooks-bin/internal/heartbeat    |
      | engram/hooks-bin/internal/limiter      |
      | engram/hooks-bin/internal/pivot        |
      | engram/hooks-bin/internal/session      |
      | engram/hooks/cmd/stop-retrospect       |
