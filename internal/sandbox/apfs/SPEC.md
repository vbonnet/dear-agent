# APFS Sandbox Provider Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The APFS provider creates macOS sandbox workspaces by cloning lower directories
into an upper directory and exposing the upper directory through a merged
symlink. It is a macOS-specific provider for copy-on-write style local
isolation where Linux union mounts are not available.

## Requirements

**APFS-01** When the provider is queried for its name, the system shall return `apfs-reflink`.

**APFS-02** When creating a sandbox, the system shall reject requests with an empty session ID, no lower directories, missing lower directories, or an empty workspace directory using structured sandbox errors.

**APFS-03** When a valid sandbox request is created, the system shall create an `upper` directory under the requested workspace directory.

**APFS-04** When lower directories are cloned, the system shall try APFS `cp -c -R` cloning first and fall back to recursive copy only when the clone operation reports unsupported clonefile semantics.

**APFS-05** When multiple lower directories are cloned, the system shall place them under deterministic `repo<N>` directories inside the sandbox upper directory.

**APFS-06** When clone setup succeeds, the system shall expose the merged path as a symlink to the sandbox upper directory.

**APFS-07** When secrets are provided, the system shall write them only to `upper/.env` with owner-only permissions.

**APFS-08** When sandbox creation fails after partial setup, the system shall remove the requested workspace directory before returning the error.

**APFS-09** When a sandbox is destroyed, the system shall remove the workspace directory and remove the sandbox from the active provider registry.

**APFS-10** When a sandbox is validated, the system shall require an active registry entry, an existing merged symlink, and an existing symlink target.

**APFS-11** When a request names a working directory inside lower directory number N, the provider shall return `merged/repoN/<relative-directory>` as the harness working directory.

## BDD Traceability

- `agm/test/bdd/features/sandbox_provider_guardrails.feature` enforces that this
  package keeps co-located SPEC coverage and that the SPEC points back to the
  executable guardrail.
