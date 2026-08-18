# SPEC: wayfinder/cmd/wayfinder-session/internal/archive/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/review/SPEC.md
Feature: Wayfinder lifecycle guardrails
  Wayfinder archive and review packages should carry
  executable SPEC traceability so lifecycle safety and review gates do not drift
  away from the implementation.

  Scenario Outline: Wayfinder lifecycle packages declare SPEC coverage
    Given Wayfinder lifecycle package "<package>" is configured
    When AGM validates Wayfinder lifecycle package coverage
    Then Wayfinder lifecycle package "<package>" should have a co-located SPEC

    Examples:
      | package                                            |
      | wayfinder/cmd/wayfinder-session/internal/archive   |
      | wayfinder/cmd/wayfinder-session/internal/review    |
