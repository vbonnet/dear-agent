# SPEC: agm/internal/ops/wtpolicy/SPEC.md
# RELATED-SPEC: agm/internal/nochecks/SPEC.md
# RELATED-SPEC: internal/safegit/SPEC.md
# RELATED-SPEC: agm/internal/orphan/SPEC.md
# RELATED-SPEC: agm/internal/orphanpr/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/config/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/daemon/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/intake/SPEC.md
# RELATED-SPEC: agm/internal/sentinel/tmux/SPEC.md
# RELATED-SPEC: agm/internal/skipdetect/SPEC.md
Feature: AGM supervision and recovery guardrails
  Recovery automation must retain executable SPEC traceability so unknown PR,
  process, worktree, intake, and tmux state fails conservatively instead of
  becoming destructive cleanup or silent skipped verification.

  Scenario Outline: AGM supervision packages declare SPEC coverage
    Given AGM supervision package "<package>" is configured
    When AGM validates supervision package coverage
    Then AGM supervision package "<package>" should have a co-located SPEC

    Examples:
      | package                      |
      | agm/internal/nochecks        |
      | agm/internal/orphan          |
      | agm/internal/orphanpr        |
      | agm/internal/ops/wtpolicy    |
      | agm/internal/sentinel/config |
      | agm/internal/sentinel/daemon |
      | agm/internal/sentinel/intake |
      | agm/internal/sentinel/tmux   |
      | agm/internal/skipdetect      |

  Scenario: Sentinel monitoring stays on its configured tmux socket
    Given sentinel monitoring owns an explicit tmux socket
    When AGM validates sentinel tmux isolation
    Then sentinel discovery should use only the configured socket
    And nested AGM recovery commands should inherit the configured socket
    And sentinel lifecycle tests should not inspect ambient tmux sessions

  Scenario: Missing-check recovery requires complete provider evidence
    Given no-check recovery can mutate a pull request branch
    When AGM validates no-check provider completeness
    Then required-check policy should use the shared layered owner
    And check-run reads should consume every provider page
    And policy failures should prevent trigger calls
    And unreadable check runs should remain indeterminate

  Scenario: Missing-check recovery resolves policy per actual pull request base
    Given no-check recovery scans pull requests across bases
    When AGM validates no-check provider completeness
    Then each non-draft pull request should use its actual base policy
    And branch selection should be an optional verified filter
    And every non-draft base policy should preflight before check-run reads
    And policy preflight should use one total deadline
    And draft pull requests should require no policy or check-run reads
    And pull request listings should require known draft state
    And pull request listings should honor a positive operator limit
    And draft output should distinguish listed from eligible pull requests
    And scan output should report the explicit base filter
    And stuck evidence should report the actual pull request base

  Scenario: Missing-check retriggers revalidate the mutation target
    Given no-check recovery can mutate a pull request branch
    When AGM validates no-check provider completeness
    Then retrigger should revalidate current pull request identity
    And stale or forked retriggers should stop before mutation
    And retrigger should recheck whether CI already appeared
    And caller cancellation should stop later trigger calls
    And retrigger dry-run should validate without mutation
    And trigger documentation should preserve snapshot boundaries
