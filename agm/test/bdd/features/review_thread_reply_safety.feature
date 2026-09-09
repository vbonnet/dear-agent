# SPEC: cmd/resolve-review-threads/SPEC.md
Feature: Review thread reply safety
  Review reply content must remain exact data throughout thread resolution.

  Scenario: Review reply bodies remain exact non-executable data
    When AGM runs the review reply data-only regressions
    Then reply-resolve should require a body-file source
    And invalid body sources should fail before GitHub mutation
    And oversized and endless body sources should be bounded before GitHub mutation
    And a replaced named source should fail without blocking
    And GitHub should receive the exact reply bytes without shell evaluation
    And failed provider diagnostics should not echo the reply body
    And body-free provider diagnostics should remain available
    And retry guidance should reuse the body-file source without rendering its content
