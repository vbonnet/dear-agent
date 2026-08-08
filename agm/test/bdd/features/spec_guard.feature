# SPEC: internal/specguard/SPEC.md
Feature: Provider-neutral SPEC guard result
  Local and CI callers receive one deterministic disposition. Semantic checks
  use immutable Git objects, while staged admission inspects only bounded
  worktree path/status metadata so dirty governed files cannot be skipped.
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
