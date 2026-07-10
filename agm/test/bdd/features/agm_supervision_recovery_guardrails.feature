# SPEC: agm/internal/ops/wtpolicy/SPEC.md
# RELATED-SPEC: agm/internal/nochecks/SPEC.md
# RELATED-SPEC: agm/internal/orphan/SPEC.md
# RELATED-SPEC: agm/internal/orphanpr/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/config/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/intake/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/tmux/SPEC.md
# RELATED-SPEC: agm/internal/skipdetect/SPEC.md
Feature: AGM supervision and recovery guardrails
  Recovery automation must retain executable SPEC traceability so unknown PR,
  process, worktree, intake, and tmux state fails conservatively instead of
  becoming destructive cleanup or silent skipped verification.

  Scenario Outline: AGM supervision packages declare SPEC coverage
    Given AGM supervision package "<package>" is configured
    When AGM validates supervision package coverage
    Then AGM supervision package "<package>" should have a co-located SPEC

    Examples:
      | package                      |
      | agm/internal/nochecks        |
      | agm/internal/orphan          |
      | agm/internal/orphanpr        |
      | agm/internal/ops/wtpolicy    |
      | agm/internal/sentinel/config |
      | agm/internal/sentinel/intake |
      | agm/internal/sentinel/tmux   |
      | agm/internal/skipdetect      |
