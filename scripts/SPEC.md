# Repository Script Specification

<!-- Last audited at: 2026-07-20 -->

## EARS Requirements

**REPO-SCRIPT-01** When a repository maintenance script runs, the system shall validate its prerequisites and operate only on the declared repository scope.

**REPO-SCRIPT-02** If a build, synchronization, installation, or verification step fails, the system shall preserve the failure and avoid a false success message.

**REPO-SCRIPT-03** When local preflight runs repository tests, the system shall use the same explicit test timeout as required CI.

**REPO-SCRIPT-04** When stale-worktree removal fails, the system shall preserve the associated local and remote branches and report the removal failure.

**REPO-SCRIPT-05** When the GOBIN guard runs and the Go toolchain bin directory or its sentinel binary is absent, the system shall append an escalation record to the decision trail and exit non-zero.

**REPO-SCRIPT-06** When the Claude plugin installer enumerates the native marketplace, the system shall use JSON string and escape state to distinguish structural delimiters from braces, brackets, quotes, or backslashes inside strings, shall retain every direct plugin across nested component arrays and one-line formatting, and shall fail closed unless it establishes exactly one non-empty unescaped root marketplace name and a complete set of non-empty, unescaped, unique direct plugin names.

**REPO-SCRIPT-07** When the Claude plugin installer cannot install or update any declared plugin, the system shall stop non-zero and shall not print its success message, so success means the complete declared plugin set was processed without a known stale or missing member.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/local_development_guardrails.feature`

## Test Traceability

- `tests/bats/install-claude-plugins.bats` verifies complete installer enumeration against independent expected names, nested component arrays, and JSON strings containing structural-looking delimiters.
