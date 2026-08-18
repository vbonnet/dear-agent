# Repository Git Hook Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Post-merge automation contracts exercised by repository integration tests.

## EARS Requirements

**GITHOOK-01** When a post-merge hook runs on the default branch, the system shall perform its configured stale-session sweep.

**GITHOOK-02** When a post-merge hook runs on a feature branch or with its opt-out enabled, the system shall skip default-branch automation.

**GITHOOK-03** When relevant command sources change, the system shall rebuild affected binaries from the authoritative default-branch source and install them atomically.

**GITHOOK-04** When deployment manifests change on the default branch, the system shall invoke the configured deployment synchronization.

**GITHOOK-05** When a merged commit explicitly closes a tracked bead, the system shall close that bead without treating a bare identifier mention as a closure instruction.

**GITHOOK-06** When optional tools, remotes, manifests, or tracked beads are absent, the system shall complete as a no-op instead of blocking the merge lifecycle.

**GITHOOK-07** When a post-merge hook runs in a repository outside every managed root, the system shall complete without invoking any maintenance stage.

**GITHOOK-08** When AGM pair provenance is tested, the system shall capture the build directory revision and single argument following `-ldflags` for both builds, require both values to match origin trunk and its exact Version, GitCommit, BuildDate, and BuiltBy profile despite a divergent local HEAD, and prove that unavailable detached checkout, malformed or overlength revision, or failed UTC commit-time resolution runs no pair build and preserves both installed files.

**GITHOOK-09** When a detached source checkout succeeds but its diagnostic cannot write to standard error, the system shall retain the detached checkout and build from origin trunk instead of falling back to a divergent in-place working tree.

## Test Traceability

- Package tests: `tests/githooks/post_merge_test.go`, `tests/githooks/post_merge_provenance_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
