# SPEC: tools/docref-lint/SPEC.md
Feature: Living-document reference linting
  A document that names a repository artifact in backticks is claiming that
  artifact exists. The lint turns that claim into a checked fact, so a living
  document cannot promise a file the tree does not carry.

  Scenario: Only backticked repository paths are read as claims
    When AGM runs the docref-lint classifier regressions
    Then only backticked known-prefix paths should be treated as claims

  Scenario: A claim about a missing artifact is reported as a finding
    When AGM runs the docref-lint scan regressions
    Then a reference to a missing artifact should be reported as a finding
