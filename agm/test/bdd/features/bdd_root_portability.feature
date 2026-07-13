# SPEC: agm/test/bdd/steps/SPEC.md
Feature: BDD repository root portability
  BDD enforcement must run from nested package directories and binaries built
  with trimmed source paths.

  Scenario: Resolve the checkout from nested package execution
    Given BDD tests execute from a nested package directory
    When AGM resolves the BDD repository root
    Then the resolver should find the checkout without compiler source paths
