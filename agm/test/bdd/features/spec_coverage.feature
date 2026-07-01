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
    And every parity SPEC should pass strict EARS lint

  Scenario: Parity-critical specs have completed audit markers
    Given AGM parity coverage requirements
    When AGM validates parity SPEC and BDD coverage
    Then every parity SPEC should have a completed audit marker

  Scenario: Parity feature files are registered in the coverage matrix
    Given AGM parity coverage requirements
    When AGM validates parity SPEC and BDD coverage
    Then every parity BDD feature should be registered in the coverage matrix

  Scenario: BDD catalog reflects executable feature files
    Given AGM parity coverage requirements
    When AGM validates parity SPEC and BDD coverage
    Then every executable BDD feature should be listed in the BDD catalog
    And every BDD catalog feature reference should exist

  Scenario: Changed production Go packages carry co-located specs
    Given AGM parity coverage requirements
    When AGM validates changed Go package SPEC coverage
    Then changed production Go packages should have co-located SPEC.md files
    And changed production Go package SPEC.md files should pass strict EARS lint
