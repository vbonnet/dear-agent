# SPEC: cmd/ears-lint/SPEC.md
# RELATED-SPEC: cmd/ears-to-bdd/SPEC.md
# RELATED-SPEC: cmd/coverage-ratchet/SPEC.md
# RELATED-SPEC: cmd/test-affected/SPEC.md
# RELATED-SPEC: cmd/repo-health/SPEC.md
# RELATED-SPEC: cmd/structural-health/SPEC.md
# RELATED-SPEC: cmd/src-health/SPEC.md
Feature: Quality command guardrails
  Repo quality and SPEC/BDD command packages should carry executable SPEC
  traceability because they enforce the same quality gates that protect parity
  work.

  Scenario Outline: Quality command packages declare SPEC coverage
    Given quality command package "<command>" is configured
    When AGM validates quality command package coverage
    Then quality command package "<command>" should have a co-located SPEC

    Examples:
      | command               |
      | cmd/ears-lint         |
      | cmd/ears-to-bdd       |
      | cmd/coverage-ratchet  |
      | cmd/test-affected     |
      | cmd/repo-health       |
      | cmd/structural-health |
      | cmd/src-health        |

  Scenario: Repo health follows tag-free BDD enforcement
    When repo health measures executable BDD discovery
    Then repo health should follow the tag-free BDD enforcement policy

  Scenario: Repo health includes canonical extensionless build files
    When repo health measures implementation source coverage
    Then repo health should recognize canonical Dockerfile and Makefile names

  Scenario Outline: Quality command specifications bound external work
    Given quality command package "<command>" is configured
    When AGM validates quality command package coverage
    Then quality command package SPEC should declare requirement "<requirement>" containing "<contract>"

    Examples:
      | command               | requirement         | contract                   |
      | cmd/coverage-ratchet  | COVERAGE-RATCHET-01 | minimum statement coverage |
      | cmd/test-affected     | TEST-AFFECTED-08    | bounded by a timeout       |
      | cmd/src-health        | SRC-HEALTH-06       | noninteractive             |
      | cmd/repo-health       | REPO-HEALTH-05      | bounded by a timeout       |
      | cmd/repo-health       | REPO-HEALTH-11      | canonical extensionless   |
