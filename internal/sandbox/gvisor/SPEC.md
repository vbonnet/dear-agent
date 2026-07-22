# gVisor Sandbox Provider Specification

<!-- Last audited at: 2026-07-20 -->

## Overview

The gVisor provider creates Linux sandboxes backed by the `runsc` runtime. It
shares the Bubblewrap provider's workspace layout and repository resolution
strategy while delegating command isolation to callers that wrap commands with
gVisor.

## Requirements

**GVISOR-01** When the provider is queried for its name, the system shall return `gvisor`.

**GVISOR-02** When creating a sandbox, the system shall reject requests with an empty session ID, no lower directories, missing lower directories, or an empty workspace directory using structured sandbox errors.

**GVISOR-03** When `runsc` is not available in `PATH`, the system shall fail creation with an unsupported-platform sandbox error before creating sandbox state.

**GVISOR-04** When a valid sandbox request is created, the system shall create `upper`, `work`, and `merged` directories under the requested workspace directory.

**GVISOR-05** When an explicit target repository is configured or a git repository can be found under the lower directories, the system shall replace the merged directory with a git worktree on an `agm/<session>` branch.

**GVISOR-06** When no git repository can be resolved, the system shall populate the merged directory with top-level symlinks from the lower directories.

**GVISOR-07** When validating runtime availability, the system shall run `runsc --version` and require output that identifies `runsc`, without requiring privileged `runsc run` execution during provider creation.

**GVISOR-08** When secrets are provided, the system shall write them only to `upper/.env` with owner-only permissions and shell environment expansion.

**GVISOR-09** When a sandbox is destroyed, the system shall remove any created git worktree, remove the sandbox directories, and remove the sandbox from the active provider registry.

**GVISOR-10** When a sandbox is validated, the system shall require an active registry entry and an existing merged path.

**GVISOR-11** When Git refuses to remove a locked sandbox worktree during destruction, the system shall preserve the worktree, sandbox directories, and provider registry entry, return the removal error, and allow destruction to be retried after the owner unlocks the worktree.

**GVISOR-12** When Git worktree removal succeeds but sandbox directory cleanup fails during destruction, the system shall retain the provider registry entry and retry only the unfinished directory cleanup phase.

**GVISOR-13** When a request names a working directory inside a lower directory, the provider shall materialize that lower directory instead of a conflicting target repository and return `merged/<relative-directory>` as the harness working directory; if the matched lower directory is not a Git repository, the provider shall not substitute another Git repository and shall give the matched directory precedence in the symlink fallback.

## BDD Traceability

- `agm/test/bdd/features/sandbox_provider_guardrails.feature` enforces that this
  package keeps co-located SPEC coverage and that the SPEC points back to the
  executable guardrail.
