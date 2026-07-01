Feature: SPEC and BDD coverage
  AGM parity governance should keep SPEC.md files and executable BDD scenarios
  paired for every parity-critical surface. Legacy packages may still be burned
  down incrementally, but new parity features should not land without a
  SPEC-backed implementation surface.

  Scenario: Parity-critical surfaces have SPEC and BDD coverage
    Given AGM parity coverage requirements
    When AGM validates parity SPEC and BDD coverage
    Then every parity surface should have a SPEC.md
    And every parity surface should have an executable BDD feature
    And every parity SPEC should declare EARS requirements

  Scenario: Parity feature files are registered in the coverage matrix
    Given AGM parity coverage requirements
    When AGM validates parity SPEC and BDD coverage
    Then every parity BDD feature should be registered in the coverage matrix

  Scenario: Changed production Go packages carry co-located specs
    Given AGM parity coverage requirements
    When AGM validates changed Go package SPEC coverage
    Then changed production Go packages should have co-located SPEC.md files
