# Provider-Neutral SPEC Guard Specification

<!-- Last audited at: 2026-08-08 -->

**Version:** 1.7
**Status:** Draft
**Scope:** `internal/specguard`

## Purpose

The module validates changed normative specifications at one seam shared by
local workflows and CI. Semantic evidence is limited to immutable Git index or
commit-tree objects. Staged mode additionally checks bounded Git-reported
worktree path/status metadata and index visibility flags so governed unstaged,
hidden, and nonignored untracked files cannot be silently skipped, without
parsing their mutable bodies.

Repository-local terminal adapters run mutable checkout code and are
cooperative reminders and blocks, not tamper-resistant controls. Any mandatory
immutable enforcement must come from a separately reviewed changed-SPEC CI and
provider rollout. This module does not prove that rollout is deployed, has run,
or is provider-required, and no source result claims that a provider hook is
installed, registered, or running.

## EARS Requirements

**SPEC-GUARD-01** When staged validation is requested, the guard shall resolve every governed source body from the Git index object ID and shall compare the index with one pinned HEAD commit.

**SPEC-GUARD-02** When committed validation is requested with a base revision, the guard shall pin the base and HEAD commits for ancestry and change selection and shall resolve every governed source body from the pinned HEAD tree.

**SPEC-GUARD-03** When Git returns governed blob content, the guard shall verify the content against its Git object ID without consulting mutable working-tree bytes.

**SPEC-GUARD-04** When the guard invokes Git, the guard shall sanitize ambient Git configuration, shall disable replacement objects and lazy fetching, and shall bound command input, output, wall time, and descendant-process lifetime.

**SPEC-GUARD-05** When a governed path has a symlink, gitlink, tree, conflict, path escape, or other nonregular Git mode, the guard shall block validation.

**SPEC-GUARD-06** When the Git index or HEAD changes during staged evaluation, including after the complete final dirty-worktree admission sequence, the guard shall block the result as a snapshot race.

**SPEC-GUARD-07** When a SPEC.md file changes, the guard shall require identified strict EARS requirements and a repository-relative reciprocal BDD link.

**SPEC-GUARD-08** When an affected BDD feature is evaluated, the guard shall require exactly one primary neutral SPEC owner and reciprocal links for every related SPEC, shall require every concrete Scenario and Scenario Outline to contain a Then assertion, shall treat And or But after Then as assertion continuations, shall require every Scenario Outline to contain nonempty Examples, and shall require at least one such runnable scenario.

**SPEC-GUARD-09** When a changed primary SPEC owner is below a dotted harness root, a plugin root, or any `harness/<member>` or `harnesses/<member>` path, the guard shall block the result even when the root is below an `internal` or `cmd` seam or the member is absent from the pinned harness authority.

**SPEC-GUARD-10** When no governed file changes, the guard shall return `allow`; when deterministic validation fails, it shall return `block`; otherwise it shall return `reminder` for the remaining semantic review.

**SPEC-GUARD-11** When the guard emits a result, the result shall identify itself as a provider-neutral source guard and shall distinguish immutable semantic inputs, staged worktree path/status admission, and uninspected provider, installation, registration, and runtime state.

**SPEC-GUARD-12** Where descendant-process termination is unavailable, the guard shall fail closed before invoking Git.

**SPEC-GUARD-13** When repository admission begins, the guard shall pin the requested directory and every ancestor identity before repository discovery and shall revalidate those identities after the discovery command.

**SPEC-GUARD-14** When repository admission resolves the canonical worktree root, Git directory, and Git common directory, the guard shall pin those directories and every ancestor identity and shall pin the `.git` and `commondir` selector states.

**SPEC-GUARD-15** When an admitted snapshot invokes Git, the guard shall revalidate the pinned repository identities and selector states before and after the invocation and shall block if a same-path replacement, ancestor replacement, or selector retarget is observed.

**SPEC-GUARD-16** When a changed primary SPEC owner is in a bare top-level directory, the guard shall classify that directory only against the pinned closed harness authority and shall require an authority and boundary-test update when the canonical harness registry changes.

**SPEC-GUARD-17** When staged validation observes a governed tracked unstaged change or nonignored untracked path, the guard shall block with a typed finding that requires the intended contract state to be staged or the dirty path to be resolved before retrying.

**SPEC-GUARD-18** When staged dirty-worktree admission runs, the guard shall use bounded NUL-framed Git path/status output and shall block malformed, oversized, ambiguous, or raced governed path observations.

**SPEC-GUARD-19** When a repository-local terminal adapter invokes the staged guard, the adapter shall disclose that mutable checkout code provides cooperative feedback rather than tamper-resistant enforcement and shall identify a separately reviewed changed-SPEC CI and provider rollout as required for any mandatory immutable enforcement without attesting that rollout from the local result.

**SPEC-GUARD-20** When an immutable snapshot is admitted, the guard shall emit a deterministic snapshot identity that changes with the pinned staged index or committed comparison so bounded transports can scope continuation state without rereading mutable source bodies.

**SPEC-GUARD-21** When an immutable snapshot changes a `SPEC.owner` edge or adds a co-located `SPEC.md` beside an existing edge, the guard shall require the edge to belong to a directory with a regular implementation source, shall require exactly one local ownership declaration, shall require one bounded canonical repository-relative path to an existing neutral `SPEC.md`, and shall apply the same strict EARS and reciprocal executable BDD checks to that canonical target.

**SPEC-GUARD-22** Where a governed path is deleted, when the selected immutable snapshot has no surviving reciprocal BDD edge or implementation ownership edge to that path and every same-change replacement passes strict validation, the guard shall retain the deletion in its changed-path evidence and shall permit it to reach mandatory semantic retirement and stable-ID preservation review instead of blocking every deletion unconditionally.

**SPEC-GUARD-23** When a `SPEC.owner` edge is deleted, the guard shall require its surviving implementation directory and every directory represented by a surviving changed implementation source in the same immutable diff to retain a valid `SPEC.owner` edge or a permitted local `SPEC.md` replacement that passes strict contract and neutrality validation.

**SPEC-GUARD-24** When staged validation observes `assume-unchanged` or `skip-worktree` on a governed path, or those index flags change during evaluation, the guard shall block before accepting a snapshot whose dirty contract state Git may suppress. Sparse checkouts that mark governed paths `skip-worktree` are explicitly unsupported.

**SPEC-GUARD-25** When a SPEC declares reciprocal BDD traceability, the guard shall admit only safe direct-child feature paths with the shared bounded basename grammar in the repository's executable `agm/test/bdd/features` suite and shall reject nested, unparseable, documentation-example, or other non-executed `.feature` files as contract evidence.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_guard.feature`

## Package Test Traceability

- `internal/specguard/guard_test.go`
- `agm/internal/agent/specguard_boundary_test.go`
