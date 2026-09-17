# SPEC: internal/buildauthority/SPEC.md
Feature: Build authority guardrails
  The authenticated build authority owns its security-sensitive implementation
  boundary independently of any supported harness or model family.

  Scenario: Build authority retains exclusive child-wait ownership
    Given the repository production source is available for build-authority wait ownership
    When AGM scans production source for build-authority wait ownership
    Then no production package outside the exact internal/buildauthority package should import C, mutate or subscribe to SIGCHLD, or broadly or foreign-reap children
