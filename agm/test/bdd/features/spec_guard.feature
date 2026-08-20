# SPEC: internal/specguard/SPEC.md
Feature: Provider-neutral SPEC guard result
  Local and CI callers receive one deterministic disposition. Semantic checks
  use immutable Git objects, while staged admission inspects only bounded
  worktree path/status and index-flag metadata so dirty or hidden governed
  files cannot be skipped.
  Repository-local hooks run mutable checkout code and are cooperative rather
  than tamper-resistant. Any mandatory immutable enforcement requires a
  separately reviewed changed-SPEC CI and provider rollout, which local results
  do not attest. No result claims provider or runtime proof.

  Scenario Outline: Malformed requests fail closed through the shared interface
    Given malformed provider-neutral SPEC guard case "<case>"
    When the shared SPEC guard interface evaluates the request
    Then the SPEC guard result should block and disclose its source, cooperative hook, and repository identity boundaries

    Examples:
      | case                 |
      | unknown-mode         |
      | staged-with-base     |
      | committed-no-base    |

  Scenario: Contract retirement validates the surviving immutable graph
    Given governed contract deletion validation is configured
    When AGM exercises dangling deletion, live owner deletion, complete retirement, and same-change relocation
    Then only structurally owned replacement, complete retirement, or relocation should reach semantic review

  Scenario: Staged contract evidence remains visible and executable
    Given staged SPEC guard visibility validation is configured
    When AGM exercises hidden index paths and executable BDD suite admission
    Then index flags and non-executed feature links should fail closed
