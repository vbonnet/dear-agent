# SPEC: internal/sandbox/SPEC.md
# RELATED-SPEC: internal/sandbox/bubblewrap/SPEC.md
# RELATED-SPEC: internal/sandbox/apfs/SPEC.md
# RELATED-SPEC: internal/sandbox/gvisor/SPEC.md
# RELATED-SPEC: internal/sandbox/overlayfs/SPEC.md
# RELATED-SPEC: wayfinder/pkg/sandbox/SPEC.md
Feature: Sandbox provider guardrails
  Sandbox provider and Wayfinder sandbox packages should carry executable SPEC
  traceability so isolation and permissions behavior stays visible across AGM
  and Wayfinder implementations.

  Scenario Outline: Sandbox packages declare SPEC coverage
    Given sandbox provider package "<package>" is configured
    When AGM validates sandbox provider package coverage
    Then sandbox provider package "<package>" should have a co-located SPEC

    Examples:
      | package                     |
      | internal/sandbox/bubblewrap |
      | internal/sandbox/apfs       |
      | internal/sandbox/gvisor     |
      | internal/sandbox/overlayfs  |
      | wayfinder/pkg/sandbox       |

  Scenario: Sandbox provider destruction preserves retryable cleanup state
    When AGM runs the sandbox provider cleanup retry regressions
    Then failed destruction should resume at the unfinished cleanup phase

  Scenario: Sandbox sessions preserve the requested project directory
    When AGM runs the sandbox working directory regressions
    Then sandbox providers should preserve the requested project directory
    And flat Linux providers should reject host-symlink fallbacks
    And APFS should detach linked worktree Git metadata on macOS
    And AGM should route the mapped directory through the shared harness lifecycle

  Scenario: Retired pseudo-providers fail before workspace creation
    When AGM runs the retired sandbox provider regressions
    Then claudecode-worktree should be rejected before workspace creation

  Scenario: Wayfinder sandbox regressions preserve the invoking repository
    Given the invoking repository worktree inventory is captured
    When Wayfinder sandbox isolation regressions run
    Then the Wayfinder sandbox isolation regressions should pass
    And the invoking repository worktree inventory should be unchanged
