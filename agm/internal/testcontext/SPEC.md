# AGM Test Context Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**TCTX-01** When a test context is created, the system shall allocate isolated home, session, database, lock, and tmux socket paths under a run-specific temporary root.

**TCTX-02** When a test context exports its environment, the system shall include the run identifier, isolated paths, and isolated home consistently for in-process and subprocess use.

**TCTX-03** If no test run marker exists in the environment, then the system shall report that no test context can be reconstructed.

**TCTX-04** When a test context is cleaned up, the system shall remove its socket and run-specific directory tree without affecting unrelated test runs.

**TCTX-05** When a test context exports its environment, the system shall route runtime readiness and lock state through its run-specific state directory instead of the user's AGM state directory.

**TCTX-06** When a named test environment is constructed or reconstructed, the system shall reject empty, absolute, path-separated, or control-character names before deriving any filesystem path.

**TCTX-07** When a new named test environment is created, the system shall derive its paths beneath one short effective-user root shared by activation, discovery, and cleanup, with canonical entries taking precedence over legacy duplicates.

**TCTX-08** When a named environment exists beneath a retired short or host temporary root, the system shall activate that exact validated environment during reconstruction and remove its directory and socket during explicit destroy without mutating sibling paths.

**TCTX-09** When a new named environment exceeds the socket-length budget, the system shall reject creation while retaining discovery and cleanup access for a path-safe legacy name.

**TCTX-10** When the canonical short test root is resolved, created, reused, or cleaned, the system shall verify it is a real directory owned by the effective user and enforce owner-only permissions before traversing environment state beneath it.

**TCTX-11** When a retired host temporary root is considered for compatibility, the system shall require a real owner-only directory owned by the effective user before resolving or cleaning any named child.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/internal/testcontext/*_test.go`
