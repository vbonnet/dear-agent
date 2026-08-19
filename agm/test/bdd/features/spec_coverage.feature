# SPEC: internal/speccoverage/SPEC.md
# RELATED-SPEC: agm/internal/sandboxgc/SPEC.md
# RELATED-SPEC: agm/internal/dispatchstate/SPEC.md
# RELATED-SPEC: internal/repoinventory/SPEC.md
Feature: SPEC and BDD coverage
  AGM governance should keep SPEC.md files and executable BDD scenarios paired
  for every parity-critical surface and every implementation directory in the
  repository.

  Scenario: Parity-critical surfaces have SPEC and BDD coverage
    Given AGM parity coverage requirements
    When AGM validates parity SPEC and BDD coverage
    Then every parity surface should have a SPEC.md
    And every parity surface should have an executable BDD feature
    And every parity SPEC should declare EARS requirements
    And every parity SPEC should pass strict EARS lint
    And every parity SPEC should reference its executable BDD feature
    And every parity BDD feature should reference its governing SPEC.md

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

  Scenario: Executable BDD feature files declare SPEC traceability
    Given AGM parity coverage requirements
    When AGM validates parity SPEC and BDD coverage
    Then every executable BDD feature should reference a governing SPEC.md
    And every governing SPEC.md should reference its executable BDD feature

  Scenario: Changed production Go packages carry co-located specs
    Given AGM parity coverage requirements
    When AGM validates changed Go package SPEC coverage
    Then changed production Go packages should have co-located SPEC.md files
    And changed production Go package SPEC.md files should pass strict EARS lint

  Scenario: The actual repository enforces implementation coverage
    Given AGM parity coverage requirements
    When AGM validates repository-wide implementation SPEC and BDD coverage
    Then every implementation directory including canonical Dockerfile and Makefile directories should have strict co-located SPEC and reciprocal BDD coverage
    And every repository SPEC should have strict EARS and reciprocal executable BDD coverage
