# AGM Test Context Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**TCTX-01** When a test context is created, the system shall allocate isolated home, session, database, lock, and tmux socket paths under a run-specific temporary root.

**TCTX-02** When a test context exports its environment, the system shall include the run identifier, isolated paths, and isolated home consistently for in-process and subprocess use.

**TCTX-03** If no test run marker exists in the environment, then the system shall report that no test context can be reconstructed.

**TCTX-04** When a test context is cleaned up, the system shall remove its socket and run-specific directory tree without affecting unrelated test runs.

**TCTX-05** When a test context exports its environment, the system shall route runtime readiness and lock state through its run-specific state directory instead of the user's AGM state directory.

**TCTX-06** When a named test environment is constructed or reconstructed, the system shall reject empty, overlong, absolute, path-separated, or control-character names before deriving any filesystem path.

**TCTX-07** When named test environments are created or reconstructed, the system shall derive their canonical paths beneath one short root shared by discovery and cleanup.

**TCTX-08** When a named environment exists beneath the retired host temporary root, discovery and explicit destroy shall remove that exact validated legacy directory and socket without mutating sibling paths.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/internal/testcontext/*_test.go`
