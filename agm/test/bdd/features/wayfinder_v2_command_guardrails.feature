# SPEC: wayfinder/cmd/wayfinder/cmd/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/commands/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/status/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/taskmanager/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/retrospective/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/validator/SPEC.md
# RELATED-SPEC: wayfinder/hooks/cmd/stop-wayfinder-guard/SPEC.md
# RELATED-SPEC: wayfinder/cmd/wayfinder-session/internal/buildloop/SPEC.md
# RELATED-SPEC: wayfinder/coordinator/SPEC.md
# RELATED-SPEC: wayfinder/internal/analytics/SPEC.md
# RELATED-SPEC: wayfinder/internal/corpus/SPEC.md
Feature: Wayfinder V2 command guardrails
  Wayfinder V2 is the only executable workflow model. Legacy state may be read
  only by explicit migration commands and must never remain a default path.

  Scenario Outline: Canonical Wayfinder command packages declare SPEC coverage
    Given Wayfinder V2 command package "<package>" is configured
    When AGM validates Wayfinder V2 command package coverage
    Then Wayfinder V2 command package "<package>" should have a co-located SPEC

    Examples:
      | package                                                     |
      | wayfinder/cmd/wayfinder/cmd                                |
      | wayfinder/cmd/wayfinder-session/commands                   |
      | wayfinder/cmd/wayfinder-session/internal/status            |
      | wayfinder/cmd/wayfinder-session/internal/taskmanager       |
      | wayfinder/cmd/wayfinder-session/internal/retrospective     |
      | wayfinder/cmd/wayfinder-session/internal/validator         |
      | wayfinder/hooks/cmd/stop-wayfinder-guard                   |
      | wayfinder/cmd/wayfinder-session/internal/buildloop         |
      | wayfinder/coordinator                                      |
      | wayfinder/internal/analytics                               |
      | wayfinder/internal/corpus                                  |

  Scenario: Root help exposes only the canonical workflow surface
    When AGM inspects the Wayfinder root help contract
    Then Wayfinder help should name all nine canonical phases
    And Wayfinder help should expose the V2 session command
    And Wayfinder help should not expose retired V1 executors

  Scenario: Retired V1 execution paths cannot return
    When AGM audits Wayfinder command source policy
    Then retired V1 root and feature executors should be absent
    And normal Wayfinder session commands should parse only V2 status
    And non-migration Wayfinder runtime source should omit retired phase identifiers
    And unversioned phase enumeration should default to V2
