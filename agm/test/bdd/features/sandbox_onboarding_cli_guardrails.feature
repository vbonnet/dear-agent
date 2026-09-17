# SPEC: agm/cmd/agm/SPEC.md
# RELATED-SPEC: agm/internal/sandboxonboarding/SPEC.md
Feature: Sandbox onboarding CLI rollback guardrails
  AGM should roll back the exact provider sandbox when authenticated onboarding
  cannot safely install retained-HOME output.

  @sandbox_onboarding_cli
  Scenario: Failed onboarding rolls back the provider sandbox
    When AGM runs the onboarding rollback command regression
    Then onboarding failure should preserve external paths and destroy the exact provider sandbox once with bounded cancellation-independent cleanup

  @sandbox_onboarding_cleanup_error
  Scenario: Provider cleanup failure remains visible
    When AGM runs the onboarding cleanup-error command regression
    Then provider cleanup failure should remain joined with onboarding failure
