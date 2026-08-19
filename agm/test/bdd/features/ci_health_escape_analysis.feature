# SPEC: pkg/cihealth/SPEC.md
# RELATED-SPEC: tools/ci-escape-analysis/SPEC.md
Feature: CI health escape analysis
  A failure that reaches main should be classified by how it got past
  pre-merge, and any proposal to move the producing check pre-merge should be
  priced with the prevention-versus-cure ratio rather than asserted.

  Scenario: Escape classification separates selection, gating, and scope
    When AGM runs the CI escape classification regressions
    Then each escape class should name the mechanism that fixes it

  Scenario: Prevention-versus-cure pricing refuses to guess without measurement
    When AGM runs the CI escape ROI regressions
    Then unmeasured prevention cost should not produce a placement verdict
