# Wayfinder Sandbox Package Specification

<!-- Last audited at: 2026-07-20 -->

## Overview

The Wayfinder sandbox package manages per-project Wayfinder sandbox directories,
metadata, optional git worktrees, active sandbox detection, and sandbox-aware
path resolution for sessions, costs, and temporary data.

## Requirements

**WF-SANDBOX-01** When a manager is created without an explicit base directory, the system shall default to the detected workspace projects directory joined with `sandboxes`.

**WF-SANDBOX-02** When creating a sandbox, the system shall generate a unique sandbox ID, preserve the requested display name, and initialize creation and last-used timestamps.

**WF-SANDBOX-03** When creating a sandbox directory, the system shall create the sandbox root with owner-only permissions and create `sessions`, `costs`, and `temp` subdirectories.

**WF-SANDBOX-04** When the current working directory is inside a git repository, the system shall create a git worktree for the sandbox and persist the worktree path and repository root in sandbox metadata.

**WF-SANDBOX-05** When any sandbox creation step fails, the system shall roll back previously created filesystem and worktree state before returning the error.

**WF-SANDBOX-06** When sandbox creation succeeds, the system shall write `.wayfinder-project` metadata with owner-only permissions.

**WF-SANDBOX-07** When detecting the active sandbox, the system shall walk upward from the current working directory until it finds `.wayfinder-project` or reaches the filesystem root.

**WF-SANDBOX-08** When listing sandboxes, the system shall return an empty list for a missing base directory and skip directories whose metadata cannot be read.

**WF-SANDBOX-09** When cleaning up a sandbox by name or ID, the system shall remove any recorded git worktree before removing the sandbox directory.

**WF-SANDBOX-10** When resolving paths with an active sandbox, the system shall return paths under the active sandbox directory.

**WF-SANDBOX-11** When resolving paths without an active sandbox or after active sandbox detection fails, the system shall fall back to `~/.wayfinder`.

**WF-SANDBOX-12** When tests need fresh active-sandbox detection, the system shall provide a cache reset path for the resolver.

**WF-SANDBOX-13** When cleanup encounters a registered worktree that Git refuses to remove, including a locked worktree, the system shall preserve the checkout and its sandbox metadata and return an error so cleanup can be retried safely.

**WF-SANDBOX-14** When Git worktree removal fails, the system shall not bypass Git worktree protection with direct filesystem deletion or metadata pruning.

**WF-SANDBOX-15** When sandbox tests exercise basic creation, listing, and cleanup, the system shall isolate the process working directory so the invoking repository's worktree registry remains unchanged.

## BDD Traceability

- `agm/test/bdd/features/sandbox_provider_guardrails.feature` enforces that this
  package keeps co-located SPEC coverage and that the SPEC points back to the
  executable guardrail.
- `agm/test/bdd/features/local_development_guardrails.feature` exercises cleanup
  overlapping a live safe-pr worktree transaction and the safe retry after the
  transaction releases its lock.
