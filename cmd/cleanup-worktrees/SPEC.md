# Cleanup Worktrees Requirements Specification (EARS)

<!-- Last audited at: 2026-08-09 -->

**Version**: 1.0
**Status**: Active
**Scope**: Stale git worktree cleanup CLI used by `scripts/cleanup-worktrees.sh`.

## EARS Requirements

**CLEANUP-WORKTREES-01** When invoked without exactly one repository path, the system shall reject the invocation with usage guidance.

**CLEANUP-WORKTREES-02** When the repository path is not a git checkout, the system shall fail without attempting cleanup.

**CLEANUP-WORKTREES-03** When a worktree is the main checkout, bare, detached, or explicitly preserved by basename, the system shall skip cleanup for that worktree.

**CLEANUP-WORKTREES-04** When a worktree has zero commits ahead of the target ref, the system shall classify it as stale.

**CLEANUP-WORKTREES-05** When a worktree has commits ahead of the target ref and its last commit is older than the configured max age, the system shall classify it as stale.

**CLEANUP-WORKTREES-06** When running without `--fix`, the system shall only report the cleanup commands that would run.

**CLEANUP-WORKTREES-07** When running with `--fix` and worktree removal fails, the system shall not delete the corresponding local or remote branch.

## Test Traceability

- Package behavior: `cmd/cleanup-worktrees/main.go`
- Regression coverage: `internal/safepr/worktree_test.go`
- Feature: `agm/test/bdd/features/local_development_guardrails.feature`
