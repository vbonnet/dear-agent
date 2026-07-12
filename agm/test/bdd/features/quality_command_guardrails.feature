# SPEC: cmd/ears-lint/SPEC.md
# RELATED-SPEC: cmd/ears-to-bdd/SPEC.md
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
      | cmd/test-affected     |
      | cmd/repo-health       |
      | cmd/structural-health |
      | cmd/src-health        |

  Scenario: Repo health follows tag-free BDD enforcement
    When repo health measures executable BDD discovery
    Then repo health should follow the tag-free BDD enforcement policy

  Scenario Outline: Quality command specifications bound external work
    Given quality command package "<command>" is configured
    When AGM validates quality command package coverage
    Then quality command package SPEC should declare requirement "<requirement>" containing "<contract>"

    Examples:
      | command           | requirement      | contract                  |
      | cmd/test-affected | TEST-AFFECTED-08 | bounded by a timeout      |
      | cmd/src-health    | SRC-HEALTH-06    | noninteractive            |
      | cmd/repo-health   | REPO-HEALTH-05   | bounded by a timeout      |
