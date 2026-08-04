# SPEC: wayfinder/cmd/wayfinder/cmd/SPEC.md
# RELATED-SPEC: wayfinder/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/commands/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/status/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/taskmanager/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/retrospective/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/validator/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/statusread/SPEC.md
# RELATED-SPEC: wayfinder/hooks/cmd/stop-wayfinder-guard/SPEC.md
# RELATED-SPEC: wayfinder/coordinator/SPEC.md
# RELATED-SPEC: wayfinder/internal/analytics/SPEC.md
Feature: Canonical Wayfinder command guardrails
  Wayfinder schema 2.0 is the only executable workflow model. Unsupported
  historical state must fail closed and stay outside the installed runtime.

  Scenario Outline: Canonical Wayfinder command packages declare SPEC coverage
    Given Wayfinder command package "<package>" is configured
    When AGM validates Wayfinder command package coverage
    Then Wayfinder command package "<package>" should have a co-located SPEC

    Examples:
      | package                                                     |
      | wayfinder/cmd/wayfinder/cmd                                |
      | wayfinder/cmd/wayfinder-session/commands                   |
      | wayfinder/cmd/wayfinder-session/internal/status            |
      | wayfinder/cmd/wayfinder-session/internal/taskmanager       |
      | wayfinder/cmd/wayfinder-session/internal/retrospective     |
      | wayfinder/cmd/wayfinder-session/internal/validator         |
      | wayfinder/cmd/wayfinder-session/statusread                 |
      | wayfinder/hooks/cmd/stop-wayfinder-guard                   |
      | wayfinder/coordinator                                      |
      | wayfinder/internal/analytics                               |

  Scenario: Root help exposes only the canonical workflow surface
    When AGM inspects the Wayfinder root help contract
    Then Wayfinder help should name all nine canonical phases
    And Wayfinder help should expose the canonical session command
    And Wayfinder help should not expose retired root executors
    And Wayfinder help should not expose retired compatibility commands

  Scenario: Retired execution paths cannot return
    When AGM audits Wayfinder command source policy
    Then retired root and feature executors should be absent
    And all Wayfinder session commands should parse only schema 2.0 status
    And Wayfinder active corpus should omit retired phase identifiers
    And retired external Wayfinder validators should be absent
    And active command guidance should use the canonical entrypoint
    And Wayfinder phase enumeration should expose the nine named phases
    And Wayfinder plugin should expose one root skill
