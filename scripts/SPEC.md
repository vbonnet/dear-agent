# Repository Script Specification

<!-- Last audited at: 2026-07-20 -->

## EARS Requirements

**REPO-SCRIPT-01** When a repository maintenance script runs, the system shall validate its prerequisites and operate only on the declared repository scope.

**REPO-SCRIPT-02** If a build, synchronization, installation, or verification step fails, the system shall preserve the failure and avoid a false success message.

**REPO-SCRIPT-03** When local preflight runs repository tests, the system shall use the same explicit test timeout as required CI.

**REPO-SCRIPT-04** When stale-worktree removal fails, the system shall preserve the associated local and remote branches and report the removal failure.

**REPO-SCRIPT-05** When the GOBIN guard runs and the Go toolchain bin directory or its sentinel binary is absent, the system shall append an escalation record to the decision trail and exit non-zero.

**REPO-SCRIPT-06** When the Claude plugin installer enumerates the native marketplace, the system shall retain every top-level plugin after a plugin that declares a nested component array and shall not treat that nested array as the end of the plugin catalog.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- `tests/bats/install-claude-plugins.bats` verifies complete installer enumeration across nested component arrays.
