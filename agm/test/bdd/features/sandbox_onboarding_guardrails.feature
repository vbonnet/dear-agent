# SPEC: agm/internal/sandboxonboarding/SPEC.md
Feature: Sandbox onboarding output guardrails
  Sandbox onboarding should install instructions only beneath authenticated
  retained-HOME directories and fail closed when installation cannot safely
  commit.

  @sandbox_onboarding_home
  Scenario: Onboarding ignores ambient HOME changes
    When AGM runs the retained HOME independence regression
    Then ambient HOME drift should not redirect onboarding output

  @sandbox_onboarding_module
  Scenario: Authenticated onboarding rejects hostile output nodes
    When AGM runs the hostile onboarding namespace regressions
    Then retained HOME output should reject hostile namespace entries without a skipped proof
