# SPEC: wayfinder/cmd/wayfinder-session/internal/status/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/phasegraph/SPEC.md
Feature: Wayfinder status guardrails
  Wayfinder's canonical V2 status and phase dependency packages should carry
  executable SPEC traceability so V2 workflow rules do not drift silently.

  Scenario Outline: Wayfinder core packages declare SPEC coverage
    Given Wayfinder core package "<package>" is configured
    When AGM validates Wayfinder core package coverage
    Then Wayfinder core package "<package>" should have a co-located SPEC

    Examples:
      | package                                                   |
      | wayfinder/cmd/wayfinder-session/internal/status           |
      | wayfinder/cmd/wayfinder-session/internal/phasegraph       |
