# Wayfinder Git Integration Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Scoped commits and worktree checks for canonical session transitions.

## EARS Requirements

**WAYFINDER-GIT-01** When a Git integrator is created, the system shall scope all operations to the configured project directory.

**WAYFINDER-GIT-02** When session initialization or a phase transition changes marker files, the system shall commit only the canonical status, history, and phase artifacts.

**WAYFINDER-GIT-03** When DESIGN completes, the system shall include the reviewed architecture and ADR files in the scoped phase commit.

**WAYFINDER-GIT-04** When no tracked transition files changed or the project is not a Git repository, the system shall complete commit helpers as non-destructive no-ops.

**WAYFINDER-GIT-05** When uncommitted files are queried, the system shall report only files contained by the project directory.

**WAYFINDER-GIT-06** When a rewind changes canonical status, history, or retrospective markers in a Git repository, the system shall commit those files without sweeping unrelated staged changes.

**WAYFINDER-GIT-07** When a repository ignore rule matches a canonical Wayfinder lifecycle marker, the system shall force-stage that owned marker while continuing to honor ignore rules for user-authored phase artifacts.

**WAYFINDER-GIT-07** When modified source files are queried, the system shall distinguish source code from Wayfinder artifacts and unrelated paths.

**WAYFINDER-GIT-08** When a worktree lifecycle is exercised, the system shall prevent conflicting paths and preserve repository integrity.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/git/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
