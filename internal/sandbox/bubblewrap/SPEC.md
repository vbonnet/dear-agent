# Bubblewrap Sandbox Provider Specification

<!-- Last audited at: 2026-07-20 -->

## Overview

The Bubblewrap provider creates Linux namespace sandboxes for AGM sessions using
`bwrap`. It requires a per-session git worktree so agents can edit and commit in
an isolated branch without writes traversing host-repository symlinks.

## Requirements

**BWRAP-01** When the provider is queried for its name, the system shall return `bubblewrap`.

**BWRAP-02** When creating a sandbox, the system shall reject requests with an empty session ID, no lower directories, missing lower directories, or an empty workspace directory using structured sandbox errors.

**BWRAP-03** When `bwrap` is not available in `PATH`, the system shall fail creation with an unsupported-platform sandbox error before creating sandbox state.

**BWRAP-04** When a valid sandbox request is created, the system shall create `upper`, `work`, and `merged` directories under the requested workspace directory.

**BWRAP-05** When an explicit target repository is configured or a git repository can be resolved from the lower directories, the system shall replace the merged directory with a git worktree on an `agm/<session>` branch.

**BWRAP-06** When no private git worktree can be materialized, the system shall fail with a structured mount error, remove partial sandbox directories, and never expose host files through a symlink-populated merged directory.

**BWRAP-07** When Bubblewrap arguments are built for execution, the system shall mount lower directories read-only, bind the writable upper directory, isolate all namespaces by default, and include `--die-with-parent`.

**BWRAP-08** When provider-level or request-level network sharing is enabled, the system shall add `--share-net`; otherwise network access shall remain isolated by default.

**BWRAP-09** When secrets are provided, the system shall write them only to `upper/.env` with owner-only permissions and shell environment expansion.

**BWRAP-10** When a sandbox is destroyed, the system shall remove any created git worktree, remove the sandbox directories, and remove the sandbox from the active provider registry.

**BWRAP-11** When a sandbox is validated, the system shall require an active registry entry and an existing merged path.

**BWRAP-12** When Git refuses to remove a locked sandbox worktree during destruction, the system shall preserve the worktree, sandbox directories, and provider registry entry, return the removal error, and allow destruction to be retried after the owner unlocks the worktree.

**BWRAP-13** When Git worktree removal succeeds but sandbox directory cleanup fails during destruction, the system shall retain the provider registry entry and retry only the unfinished directory cleanup phase.

**BWRAP-14** When a request names a working directory inside a lower directory, the provider shall materialize that lower directory instead of a conflicting target repository and return `merged/<relative-directory>` as the harness working directory; if the matched lower directory is not a Git repository, the provider shall reject creation instead of substituting another repository or exposing the host directory through symlinks.

**BWRAP-15** When private Git worktree creation fails after a repository is resolved, the system shall return a structured mount error that preserves the underlying Git failure.

**BWRAP-16** When no explicit target repository is requested, repository discovery shall be constrained to the request's lower directories and shall never materialize a repository discovered only through machine-level AGM configuration.

## BDD Traceability

- `agm/test/bdd/features/sandbox_provider_guardrails.feature` enforces that this
  package keeps co-located SPEC coverage and that the SPEC points back to the
  executable guardrail.
