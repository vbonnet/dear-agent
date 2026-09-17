# SPEC: agm/internal/circuitbreaker/SPEC.md
# RELATED-SPEC: agm/internal/ops/SPEC.md
Feature: Sandbox volume admission
  Disk admission must follow the configured parent sandbox volume even before
  the per-session workspace exists and must re-resolve that path on each probe.

  Scenario: Initial admission follows the configured sandbox volume
    When AGM validates configured sandbox volume selection
    Then initial admission should use the configured volume
    And each disk probe should re-resolve the planned path and fail closed on ambiguity or symlinks
